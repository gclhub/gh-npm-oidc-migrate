package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeContent_StaticToken(t *testing.T) {
	workflow := `
name: Publish
on:
  release:
    types: [created]
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.JobName != "publish" {
		t.Errorf("expected job name 'publish', got %q", f.JobName)
	}
	if f.EnvVar != "NODE_AUTH_TOKEN" {
		t.Errorf("expected env var 'NODE_AUTH_TOKEN', got %q", f.EnvVar)
	}
	if f.HasIDToken {
		t.Error("expected HasIDToken to be false")
	}
}

func TestAnalyzeContent_AlreadyOIDC(t *testing.T) {
	workflow := `
name: Publish
on:
  release:
    types: [created]
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v4
      - run: npm publish
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for OIDC workflow, got %d", len(findings))
	}
}

func TestAnalyzeContent_NoNpmPublish(t *testing.T) {
	workflow := `
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: npm test
`
	findings, err := AnalyzeContent([]byte(workflow), "ci.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for non-publish workflow, got %d", len(findings))
	}
}

func TestAnalyzeContent_JobLevelEnv(t *testing.T) {
	workflow := `
name: Publish
on:
  release:
    types: [created]
jobs:
  publish:
    runs-on: ubuntu-latest
    env:
      NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: npm publish
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].EnvVar != "NODE_AUTH_TOKEN" {
		t.Errorf("expected env var NODE_AUTH_TOKEN, got %q", findings[0].EnvVar)
	}
}

func TestAnalyzeContent_WorkflowLevelEnv(t *testing.T) {
	workflow := `
name: Publish
on:
  release:
    types: [created]
env:
  NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: npm publish
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestAnalyzeContent_MultipleJobs(t *testing.T) {
	workflow := `
name: Release
on:
  release:
    types: [created]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
  publish:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish --tag beta
        env:
          NPM_TOKEN: ${{ secrets.NPM_PUBLISH_TOKEN }}
`
	findings, err := AnalyzeContent([]byte(workflow), "release.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
}

func TestAnalyzeContent_NPMTokenEnvVar(t *testing.T) {
	workflow := `
name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish
        env:
          NPM_TOKEN: ${{ secrets.NPM_PUBLISH_TOKEN }}
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].EnvVar != "NPM_TOKEN" {
		t.Errorf("expected env var NPM_TOKEN, got %q", findings[0].EnvVar)
	}
}

func TestAnalyzeContent_HasIDTokenButAlsoStaticToken(t *testing.T) {
	workflow := `
name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
`
	findings, err := AnalyzeContent([]byte(workflow), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if !findings[0].HasIDToken {
		t.Error("expected HasIDToken to be true")
	}
}

func TestAnalyze_Directory(t *testing.T) {
	dir := t.TempDir()

	// Write a workflow file with static token
	err := os.WriteFile(filepath.Join(dir, "publish.yml"), []byte(`
name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write a workflow file without npm publish
	err = os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(`
name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	report, err := Analyze(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.FilesScanned != 2 {
		t.Errorf("expected 2 files scanned, got %d", report.FilesScanned)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(report.Findings))
	}
}

func TestAnalyze_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	report, err := Analyze(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.FilesScanned != 0 {
		t.Errorf("expected 0 files scanned, got %d", report.FilesScanned)
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(report.Findings))
	}
}

func TestAnalyze_MissingDir(t *testing.T) {
	_, err := Analyze("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestAnalyzeContent_InvalidYAML(t *testing.T) {
	_, err := AnalyzeContent([]byte("{{invalid yaml"), "bad.yml")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestAnalyzeContent_EmptyFile(t *testing.T) {
	findings, err := AnalyzeContent([]byte(""), "empty.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}
