package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version string
	baseDir string
)

// SetVersion sets the version string
func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "td",
	Short: "Local task and session management CLI",
	Long: `td - A minimalist local task and session management CLI designed for AI-assisted development workflows.

Optimized for session continuity—capturing working state so new context windows can resume where previous ones stopped.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initBaseDir)
}

func initBaseDir() {
	var err error
	baseDir, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
}

// getBaseDir returns the base directory for the project
func getBaseDir() string {
	return baseDir
}
