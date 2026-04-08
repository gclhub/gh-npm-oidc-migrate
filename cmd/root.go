package cmd

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command for the gh-npm-oidc-migrate extension.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "npm-oidc-migrate",
		Short: "Migrate npm publishing workflows from static tokens to OIDC",
		Long: `gh npm-oidc-migrate is a GitHub CLI extension that analyzes and converts
npm publishing GitHub Actions workflows from using fixed NPM tokens to
using OIDC-based trusted publishing with ephemeral credentials.

Usage:
  gh npm-oidc-migrate analyze [path]    Scan workflows for static NPM token usage
  gh npm-oidc-migrate migrate [path]    Convert workflows to OIDC-based publishing`,
	}

	rootCmd.AddCommand(newAnalyzeCmd())
	rootCmd.AddCommand(newMigrateCmd())

	return rootCmd
}
