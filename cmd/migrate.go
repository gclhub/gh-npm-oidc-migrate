package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gclhub/gh-npm-oidc-migrate/internal/analyzer"
	"github.com/gclhub/gh-npm-oidc-migrate/internal/migrator"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate [path]",
		Short: "Migrate workflows from static NPM tokens to OIDC",
		Long: `Convert GitHub Actions workflow files from using static NPM tokens
to OIDC-based trusted publishing with ephemeral credentials.

Changes include:
  - Adding 'permissions: id-token: write' to publish jobs
  - Removing NODE_AUTH_TOKEN / NPM_TOKEN env vars that reference secrets

Use --dry-run to preview changes without writing files.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cmd, args, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without writing files")
	return cmd
}

func runMigrate(cmd *cobra.Command, args []string, dryRun bool) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	workflowDir := resolveWorkflowDir(dir)

	if _, err := os.Stat(workflowDir); os.IsNotExist(err) {
		return fmt.Errorf("workflow directory not found: %s", workflowDir)
	}

	// First analyze to find what needs migration
	report, err := analyzer.Analyze(workflowDir)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if len(report.Findings) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "✅ No static NPM token usage found. Nothing to migrate.")
		return nil
	}

	// Collect unique files that need migration
	files := uniqueFiles(report.Findings)

	fmt.Fprintf(cmd.OutOrStdout(), "Migrating %d workflow file(s)...\n\n", len(files))

	var totalChanges int
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", file, err)
		}

		result, err := migrator.Migrate(data, file)
		if err != nil {
			return fmt.Errorf("migrating %s: %w", file, err)
		}

		if !result.Modified {
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "📝 %s\n", file)
		for _, change := range result.Changes {
			fmt.Fprintf(cmd.OutOrStdout(), "   • %s\n", change)
			totalChanges++
		}
		fmt.Fprintln(cmd.OutOrStdout())

		if !dryRun {
			if err := os.WriteFile(file, result.Output, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", file, err)
			}
		}
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run complete. %d change(s) would be applied.\n", totalChanges)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Migration complete. %d change(s) applied.\n", totalChanges)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	printPostMigrationSteps(cmd)

	return nil
}

func uniqueFiles(findings []analyzer.Finding) []string {
	seen := make(map[string]bool)
	var files []string
	for _, f := range findings {
		abs, err := filepath.Abs(f.File)
		if err != nil {
			abs = f.File
		}
		if !seen[abs] {
			seen[abs] = true
			files = append(files, f.File)
		}
	}
	return files
}

func printPostMigrationSteps(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout(), "📋 Additional setup required:")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "  1. Configure Trusted Publishing on npmjs.com:")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Go to your package settings at https://www.npmjs.com/package/<your-package>/access")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Under 'Trusted Publisher', select 'GitHub Actions'")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Enter your repository owner, name, and workflow filename")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Save the configuration")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "  2. Ensure npm CLI compatibility:")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Use npm v11.5.1+ and Node.js 22+ for full OIDC support")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Or add 'npm install -g npm@latest' step to your workflow")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "  3. Remove the old NPM_TOKEN secret from your repository:")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Go to Settings → Secrets and variables → Actions")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Delete the NPM_TOKEN (or NODE_AUTH_TOKEN) secret")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "  4. Verify the migration:")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Create a test release or manually trigger the workflow")
	fmt.Fprintln(cmd.OutOrStdout(), "     - Check the npm package page for the provenance badge")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "  For more information, see: https://docs.npmjs.com/trusted-publishers")
}
