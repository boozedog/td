package cmd

import (
	"github.com/spf13/cobra"
)

var statsSecurityCmd = &cobra.Command{
	Use:   "security",
	Short: "View security exception log (alias for 'td security')",
	Long:  `Shows audit log of creator-approval and self-close workflow exceptions.`,
	RunE:  securityCmd.RunE,
}

func init() {
	statsCmd.AddCommand(statsSecurityCmd)

	// Copy flags from security command
	statsSecurityCmd.Flags().Bool("clear", false, "Clear the security log")
	statsSecurityCmd.Flags().Bool("json", false, "Output as JSONL")
}
