// Package analyzer scans GitHub Actions workflow files for npm publishing
// steps that use static NPM tokens instead of OIDC-based trusted publishing.
package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finding describes a single instance of static-token npm publishing detected
// in a workflow file.
type Finding struct {
	File        string // Path to the workflow file
	JobName     string // Name of the job containing the finding
	StepIndex   int    // Index of the step within the job
	StepName    string // Name or uses value of the step
	EnvVar      string // Environment variable name (e.g. NODE_AUTH_TOKEN)
	SecretRef   string // The secret reference (e.g. secrets.NPM_TOKEN)
	HasIDToken  bool   // Whether the job already has id-token: write
	Description string // Human-readable description of the finding
}

// Report is the result of analyzing one or more workflow files.
type Report struct {
	Findings     []Finding
	FilesScanned int
}

// Analyze scans all workflow files in the given directory and returns a report
// of npm publishing steps that use static tokens.
func Analyze(workflowDir string) (*Report, error) {
	report := &Report{}

	files, err := findWorkflowFiles(workflowDir)
	if err != nil {
		return nil, fmt.Errorf("finding workflow files: %w", err)
	}

	for _, file := range files {
		report.FilesScanned++
		findings, err := analyzeFile(file)
		if err != nil {
			return nil, fmt.Errorf("analyzing %s: %w", file, err)
		}
		report.Findings = append(report.Findings, findings...)
	}

	return report, nil
}

// AnalyzeContent analyzes a single workflow file's content and returns findings.
func AnalyzeContent(content []byte, filename string) ([]Finding, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	return analyzeDocument(&doc, filename)
}

func findWorkflowFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

func analyzeFile(path string) ([]Finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return AnalyzeContent(data, path)
}

func analyzeDocument(doc *yaml.Node, filename string) ([]Finding, error) {
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}

	var findings []Finding

	// Find the "jobs" key
	jobsNode := findMapValue(root, "jobs")
	if jobsNode == nil || jobsNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	// Iterate over each job
	for i := 0; i < len(jobsNode.Content)-1; i += 2 {
		jobKey := jobsNode.Content[i]
		jobValue := jobsNode.Content[i+1]
		if jobValue.Kind != yaml.MappingNode {
			continue
		}

		jobName := jobKey.Value
		hasIDToken := jobHasIDTokenPermission(root, jobValue)

		// Find steps
		stepsNode := findMapValue(jobValue, "steps")
		if stepsNode == nil || stepsNode.Kind != yaml.SequenceNode {
			continue
		}

		for stepIdx, step := range stepsNode.Content {
			if step.Kind != yaml.MappingNode {
				continue
			}

			stepName := getStepName(step)

			// Check if this step runs npm publish
			if !isNpmPublishStep(step) {
				continue
			}

			// Check for static token env vars
			envFindings := checkEnvForTokens(step, jobValue, root)
			for _, ef := range envFindings {
				findings = append(findings, Finding{
					File:       filename,
					JobName:    jobName,
					StepIndex:  stepIdx,
					StepName:   stepName,
					EnvVar:     ef.envVar,
					SecretRef:  ef.secretRef,
					HasIDToken: hasIDToken,
					Description: fmt.Sprintf(
						"Step %q in job %q uses npm publish with static token %s referencing %s",
						stepName, jobName, ef.envVar, ef.secretRef,
					),
				})
			}
		}
	}

	return findings, nil
}

type envFinding struct {
	envVar    string
	secretRef string
}

// checkEnvForTokens checks for token-related env vars at step, job, and
// workflow levels.
func checkEnvForTokens(step, job, root *yaml.Node) []envFinding {
	var findings []envFinding

	tokenEnvVars := []string{"NODE_AUTH_TOKEN", "NPM_TOKEN"}

	// Check step-level env
	findings = append(findings, checkNodeEnv(step, tokenEnvVars)...)

	// Check job-level env
	findings = append(findings, checkNodeEnv(job, tokenEnvVars)...)

	// Check workflow-level env
	findings = append(findings, checkNodeEnv(root, tokenEnvVars)...)

	return findings
}

func checkNodeEnv(node *yaml.Node, tokenVars []string) []envFinding {
	var findings []envFinding
	envNode := findMapValue(node, "env")
	if envNode == nil || envNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(envNode.Content)-1; i += 2 {
		key := envNode.Content[i].Value
		val := envNode.Content[i+1].Value

		for _, tokenVar := range tokenVars {
			if strings.EqualFold(key, tokenVar) && isSecretReference(val) {
				findings = append(findings, envFinding{
					envVar:    key,
					secretRef: val,
				})
			}
		}
	}

	return findings
}

func isNpmPublishStep(step *yaml.Node) bool {
	runNode := findMapValue(step, "run")
	if runNode != nil {
		return containsNpmPublish(runNode.Value)
	}
	return false
}

func containsNpmPublish(cmd string) bool {
	// Match "npm publish" in various forms
	lower := strings.ToLower(cmd)
	return strings.Contains(lower, "npm publish")
}

func isSecretReference(val string) bool {
	lower := strings.ToLower(strings.TrimSpace(val))
	return strings.Contains(lower, "secrets.")
}

func getStepName(step *yaml.Node) string {
	nameNode := findMapValue(step, "name")
	if nameNode != nil && nameNode.Value != "" {
		return nameNode.Value
	}
	usesNode := findMapValue(step, "uses")
	if usesNode != nil && usesNode.Value != "" {
		return usesNode.Value
	}
	runNode := findMapValue(step, "run")
	if runNode != nil {
		lines := strings.SplitN(runNode.Value, "\n", 2)
		if len(lines[0]) > 50 {
			return lines[0][:50] + "..."
		}
		return lines[0]
	}
	return fmt.Sprintf("step-%d", 0)
}

// jobHasIDTokenPermission checks whether the job or workflow already has
// id-token: write permission set.
func jobHasIDTokenPermission(root, job *yaml.Node) bool {
	// Check job-level permissions
	if hasIDTokenWrite(job) {
		return true
	}
	// Check workflow-level permissions
	return hasIDTokenWrite(root)
}

func hasIDTokenWrite(node *yaml.Node) bool {
	permsNode := findMapValue(node, "permissions")
	if permsNode == nil {
		return false
	}
	if permsNode.Kind != yaml.MappingNode {
		return false
	}
	idTokenNode := findMapValue(permsNode, "id-token")
	if idTokenNode == nil {
		return false
	}
	return strings.EqualFold(idTokenNode.Value, "write")
}

func findMapValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
