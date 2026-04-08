// Package migrator converts GitHub Actions workflow files from using static
// NPM tokens to OIDC-based trusted publishing.
package migrator

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Result describes the outcome of migrating a single workflow file.
type Result struct {
	File     string
	Modified bool
	Output   []byte
	Changes  []string
}

// Migrate takes a workflow file's content and returns a migrated version that
// uses OIDC instead of static NPM tokens.
func Migrate(content []byte, filename string) (*Result, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return &Result{File: filename}, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return &Result{File: filename}, nil
	}

	result := &Result{File: filename}

	jobsNode := findMapValue(root, "jobs")
	if jobsNode == nil || jobsNode.Kind != yaml.MappingNode {
		return result, nil
	}

	for i := 0; i < len(jobsNode.Content)-1; i += 2 {
		jobKey := jobsNode.Content[i]
		jobValue := jobsNode.Content[i+1]
		if jobValue.Kind != yaml.MappingNode {
			continue
		}

		jobName := jobKey.Value

		if !jobHasNpmPublish(jobValue) {
			continue
		}

		// 1. Ensure id-token: write permission is set on the job
		if !hasIDTokenWrite(jobValue) && !hasIDTokenWrite(root) {
			addIDTokenPermission(jobValue)
			result.Changes = append(result.Changes,
				fmt.Sprintf("Added 'permissions: id-token: write' to job %q", jobName))
			result.Modified = true
		}

		// 2. Remove static token env vars from steps, job, and mark for removal
		changes := removeTokenEnvVars(jobValue, jobName)
		if len(changes) > 0 {
			result.Changes = append(result.Changes, changes...)
			result.Modified = true
		}

		// 3. Remove workflow-level token env vars if present
		wfChanges := removeWorkflowTokenEnvVars(root)
		if len(wfChanges) > 0 {
			result.Changes = append(result.Changes, wfChanges...)
			result.Modified = true
		}
	}

	if result.Modified {
		out, err := yaml.Marshal(&doc)
		if err != nil {
			return nil, fmt.Errorf("marshaling YAML: %w", err)
		}
		result.Output = out
	}

	return result, nil
}

func jobHasNpmPublish(job *yaml.Node) bool {
	stepsNode := findMapValue(job, "steps")
	if stepsNode == nil || stepsNode.Kind != yaml.SequenceNode {
		return false
	}
	for _, step := range stepsNode.Content {
		if step.Kind != yaml.MappingNode {
			continue
		}
		runNode := findMapValue(step, "run")
		if runNode != nil && containsNpmPublish(runNode.Value) {
			return true
		}
	}
	return false
}

func addIDTokenPermission(job *yaml.Node) {
	permsNode := findMapValue(job, "permissions")
	if permsNode == nil {
		// Add a new permissions mapping to the job
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "permissions",
			Tag:   "!!str",
		}
		valueNode := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "contents", Tag: "!!str"},
				{Kind: yaml.ScalarNode, Value: "read", Tag: "!!str"},
				{Kind: yaml.ScalarNode, Value: "id-token", Tag: "!!str"},
				{Kind: yaml.ScalarNode, Value: "write", Tag: "!!str"},
			},
		}

		// Insert permissions after runs-on if possible, or at start of job
		insertIdx := findInsertIndex(job, "runs-on")
		if insertIdx == -1 {
			insertIdx = 0
		}

		newContent := make([]*yaml.Node, 0, len(job.Content)+2)
		newContent = append(newContent, job.Content[:insertIdx]...)
		newContent = append(newContent, keyNode, valueNode)
		newContent = append(newContent, job.Content[insertIdx:]...)
		job.Content = newContent
	} else if permsNode.Kind == yaml.MappingNode {
		// Permissions exist but no id-token; add it
		if findMapValue(permsNode, "id-token") == nil {
			permsNode.Content = append(permsNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "id-token", Tag: "!!str"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "write", Tag: "!!str"},
			)
		}
	}
}

// findInsertIndex returns the index after the key-value pair for the given key
// in a mapping node, or -1 if not found.
func findInsertIndex(node *yaml.Node, key string) int {
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return i + 2
		}
	}
	return -1
}

func removeTokenEnvVars(job *yaml.Node, jobName string) []string {
	var changes []string
	tokenVars := []string{"NODE_AUTH_TOKEN", "NPM_TOKEN"}

	// Remove from job-level env
	changes = append(changes, removeEnvVarsFromNode(job, tokenVars, "job "+jobName)...)

	// Remove from step-level env
	stepsNode := findMapValue(job, "steps")
	if stepsNode != nil && stepsNode.Kind == yaml.SequenceNode {
		for _, step := range stepsNode.Content {
			if step.Kind != yaml.MappingNode {
				continue
			}
			stepName := getStepName(step)
			changes = append(changes, removeEnvVarsFromNode(step, tokenVars, fmt.Sprintf("step %q", stepName))...)
		}
	}

	return changes
}

func removeWorkflowTokenEnvVars(root *yaml.Node) []string {
	tokenVars := []string{"NODE_AUTH_TOKEN", "NPM_TOKEN"}
	return removeEnvVarsFromNode(root, tokenVars, "workflow level")
}

func removeEnvVarsFromNode(node *yaml.Node, tokenVars []string, context string) []string {
	var changes []string
	envNode := findMapValue(node, "env")
	if envNode == nil || envNode.Kind != yaml.MappingNode {
		return nil
	}

	var newContent []*yaml.Node
	for i := 0; i < len(envNode.Content)-1; i += 2 {
		key := envNode.Content[i].Value
		val := envNode.Content[i+1].Value

		remove := false
		for _, tokenVar := range tokenVars {
			if strings.EqualFold(key, tokenVar) && isSecretReference(val) {
				changes = append(changes,
					fmt.Sprintf("Removed %s env var from %s (was: %s)", key, context, val))
				remove = true
				break
			}
		}
		if !remove {
			newContent = append(newContent, envNode.Content[i], envNode.Content[i+1])
		}
	}

	if len(newContent) == 0 {
		// Remove the entire env key from the parent node
		removeMapKey(node, "env")
	} else {
		envNode.Content = newContent
	}

	return changes
}

func removeMapKey(node *yaml.Node, key string) {
	var newContent []*yaml.Node
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value != key {
			newContent = append(newContent, node.Content[i], node.Content[i+1])
		}
	}
	node.Content = newContent
}

func getStepName(step *yaml.Node) string {
	nameNode := findMapValue(step, "name")
	if nameNode != nil && nameNode.Value != "" {
		return nameNode.Value
	}
	runNode := findMapValue(step, "run")
	if runNode != nil {
		lines := strings.SplitN(runNode.Value, "\n", 2)
		return lines[0]
	}
	return "unknown"
}

func containsNpmPublish(cmd string) bool {
	return strings.Contains(strings.ToLower(cmd), "npm publish")
}

func isSecretReference(val string) bool {
	lower := strings.ToLower(strings.TrimSpace(val))
	return strings.Contains(lower, "secrets.")
}

func hasIDTokenWrite(node *yaml.Node) bool {
	permsNode := findMapValue(node, "permissions")
	if permsNode == nil || permsNode.Kind != yaml.MappingNode {
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
