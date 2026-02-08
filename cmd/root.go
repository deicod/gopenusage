/*
Copyright © 2026 Darko Luketic <info@icod.de>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gopenusage",
	Short: "Track AI coding subscriptions via CLI and JSON API",
	Long:  "A Cobra CLI for querying OpenUsage-compatible providers, serving a JSON API, and querying that API from the terminal.",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
