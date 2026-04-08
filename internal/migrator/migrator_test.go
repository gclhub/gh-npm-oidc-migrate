package migrator

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMigrate_StaticTokenToOIDC(t *testing.T) {
	input := `name: Publish
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
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}
	if len(result.Changes) == 0 {
		t.Fatal("expected changes to be reported")
	}

	output := string(result.Output)

	// Should have id-token permission
	if !strings.Contains(output, "id-token") {
		t.Error("expected output to contain 'id-token'")
	}

	// Should not have NODE_AUTH_TOKEN
	if strings.Contains(output, "NODE_AUTH_TOKEN") {
		t.Error("expected output to not contain 'NODE_AUTH_TOKEN'")
	}

	// Should not have secrets.NPM_TOKEN
	if strings.Contains(output, "secrets.NPM_TOKEN") {
		t.Error("expected output to not contain 'secrets.NPM_TOKEN'")
	}

	// Should still have npm publish
	if !strings.Contains(output, "npm publish") {
		t.Error("expected output to still contain 'npm publish'")
	}

	// Verify it parses as valid YAML
	var doc yaml.Node
	if err := yaml.Unmarshal(result.Output, &doc); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
}

func TestMigrate_AlreadyOIDC(t *testing.T) {
	input := `name: Publish
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
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Modified {
		t.Fatal("expected result to not be modified for OIDC workflow")
	}
}

func TestMigrate_NoNpmPublish(t *testing.T) {
	input := `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
`
	result, err := Migrate([]byte(input), "ci.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Modified {
		t.Fatal("expected result to not be modified for non-publish workflow")
	}
}

func TestMigrate_JobLevelEnv(t *testing.T) {
	input := `name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    env:
      NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
    steps:
      - run: npm publish
`
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)
	if strings.Contains(output, "NODE_AUTH_TOKEN") {
		t.Error("expected NODE_AUTH_TOKEN to be removed")
	}
	if !strings.Contains(output, "id-token") {
		t.Error("expected id-token permission to be added")
	}
}

func TestMigrate_WorkflowLevelEnv(t *testing.T) {
	input := `name: Publish
on: push
env:
  NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish
`
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)
	if strings.Contains(output, "NODE_AUTH_TOKEN") {
		t.Error("expected NODE_AUTH_TOKEN to be removed")
	}
}

func TestMigrate_MultipleTokenVars(t *testing.T) {
	input := `name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
          NPM_TOKEN: ${{ secrets.NPM_PUBLISH_TOKEN }}
`
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)
	if strings.Contains(output, "NODE_AUTH_TOKEN") {
		t.Error("expected NODE_AUTH_TOKEN to be removed")
	}
	if strings.Contains(output, "NPM_TOKEN") {
		t.Error("expected NPM_TOKEN to be removed")
	}
}

func TestMigrate_PreservesOtherEnvVars(t *testing.T) {
	input := `name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
          CI: "true"
`
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)
	if strings.Contains(output, "NODE_AUTH_TOKEN") {
		t.Error("expected NODE_AUTH_TOKEN to be removed")
	}
	if !strings.Contains(output, "CI") {
		t.Error("expected CI env var to be preserved")
	}
}

func TestMigrate_ExistingPermissionsNoIDToken(t *testing.T) {
	input := `name: Publish
on: push
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
`
	result, err := Migrate([]byte(input), "publish.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)
	if !strings.Contains(output, "id-token") {
		t.Error("expected id-token to be added to existing permissions")
	}
	if !strings.Contains(output, "contents") {
		t.Error("expected existing contents permission to be preserved")
	}
}

func TestMigrate_InvalidYAML(t *testing.T) {
	_, err := Migrate([]byte("{{invalid yaml"), "bad.yml")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestMigrate_EmptyFile(t *testing.T) {
	result, err := Migrate([]byte(""), "empty.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Modified {
		t.Error("expected result to not be modified for empty file")
	}
}

func TestMigrate_MultipleJobs(t *testing.T) {
	input := `name: Release
on: push
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
`
	result, err := Migrate([]byte(input), "release.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Modified {
		t.Fatal("expected result to be modified")
	}

	output := string(result.Output)

	// Should still have npm test
	if !strings.Contains(output, "npm test") {
		t.Error("expected non-publish job to be preserved")
	}
}
