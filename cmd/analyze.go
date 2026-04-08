package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gclhub/gh-npm-oidc-migrate/internal/analyzer"
	"github.com/spf13/cobra"
)

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze workflows for static NPM token usage",
		Long: `Scan GitHub Actions workflow files and report any npm publish steps
that use static NPM tokens (e.g. NODE_AUTH_TOKEN from secrets).

If no path is provided, the current directory is used and workflows
are looked up in .github/workflows/.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAnalyze,
	}
	return cmd
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	workflowDir := resolveWorkflowDir(dir)

	if _, err := os.Stat(workflowDir); os.IsNotExist(err) {
		return fmt.Errorf("workflow directory not found: %s", workflowDir)
	}

	report, err := analyzer.Analyze(workflowDir)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Scanned %d workflow file(s) in %s\n\n", report.FilesScanned, workflowDir)

	if len(report.Findings) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "✅ No static NPM token usage found. Your workflows may already use OIDC or do not publish to npm.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Found %d finding(s):\n\n", len(report.Findings))
	for i, f := range report.Findings {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", i+1, f.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "     File: %s\n", f.File)
		fmt.Fprintf(cmd.OutOrStdout(), "     Job:  %s (step %d)\n", f.JobName, f.StepIndex)
		if f.HasIDToken {
			fmt.Fprintf(cmd.OutOrStdout(), "     Note: Job already has id-token: write permission\n")
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Run 'gh npm-oidc-migrate migrate' to convert these workflows to OIDC.")
	return nil
}

func resolveWorkflowDir(dir string) string {
	// If the given path already looks like a workflows directory, use it
	if filepath.Base(dir) == "workflows" {
		return dir
	}
	// If .github/workflows exists under the given dir, use that
	candidate := filepath.Join(dir, ".github", "workflows")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	// Otherwise just use the directory as-is
	return dir
}
