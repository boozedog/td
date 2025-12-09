package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/output"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [issue-id...]",
	Short: "Soft-delete one or more issues",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		for _, issueID := range args {
			// Verify issue exists
			_, err := database.GetIssue(issueID)
			if err != nil {
				output.Error("%v", err)
				continue
			}

			if err := database.DeleteIssue(issueID); err != nil {
				output.Error("failed to delete %s: %v", issueID, err)
				continue
			}

			fmt.Printf("DELETED %s\n", issueID)
		}

		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore [issue-id...]",
	Short: "Restore soft-deleted issues",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		for _, issueID := range args {
			if err := database.RestoreIssue(issueID); err != nil {
				output.Error("failed to restore %s: %v", issueID, err)
				continue
			}

			fmt.Printf("RESTORED %s\n", issueID)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(restoreCmd)
}
