package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/output"
	"github.com/marcus/td/internal/session"
	"github.com/spf13/cobra"
)

var focusCmd = &cobra.Command{
	Use:   "focus [issue-id]",
	Short: "Set the current working issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		issueID := args[0]

		// Verify issue exists
		_, err = database.GetIssue(issueID)
		if err != nil {
			output.Error("%v", err)
			return err
		}

		if err := config.SetFocus(baseDir, issueID); err != nil {
			output.Error("failed to set focus: %v", err)
			return err
		}

		fmt.Printf("FOCUSED %s\n", issueID)
		return nil
	},
}

var unfocusCmd = &cobra.Command{
	Use:   "unfocus",
	Short: "Clear focus",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		if err := config.ClearFocus(baseDir); err != nil {
			output.Error("failed to clear focus: %v", err)
			return err
		}

		fmt.Println("UNFOCUSED")
		return nil
	},
}

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show focused issue, active work, and pending reviews",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		sess, err := session.Get(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")

		result := map[string]interface{}{
			"session": sess.ID,
		}

		// Get focused issue
		focusedID, _ := config.GetFocus(baseDir)
		if focusedID != "" {
			issue, err := database.GetIssue(focusedID)
			if err == nil {
				if jsonOutput {
					result["focused"] = issue
				} else {
					fmt.Printf("FOCUSED: %s  %s  %s  %dpts  %s\n",
						issue.ID, issue.Title, issue.Priority, issue.Points, output.FormatStatus(issue.Status))
					fmt.Println()
				}
			}
		} else if !jsonOutput {
			fmt.Println("No focused issue")
			fmt.Println()
		}

		// Get in-progress issues for this session
		inProgress, _ := database.ListIssues(db.ListIssuesOptions{
			Status:      []models.Status{models.StatusInProgress},
			Implementer: sess.ID,
		})

		if len(inProgress) > 0 {
			if jsonOutput {
				result["in_progress"] = inProgress
			} else {
				fmt.Println("IN PROGRESS (this session):")
				for _, issue := range inProgress {
					fmt.Printf("  %s  %s  %s  %dpts\n",
						issue.ID, issue.Title, issue.Priority, issue.Points)
				}
				fmt.Println()
			}
		}

		// Get issues awaiting review (that this session can review)
		reviewable, _ := database.ListIssues(db.ListIssuesOptions{
			ReviewableBy: sess.ID,
		})

		if len(reviewable) > 0 {
			if jsonOutput {
				result["awaiting_review"] = reviewable
			} else {
				fmt.Println("AWAITING YOUR REVIEW:")
				for _, issue := range reviewable {
					fmt.Printf("  %s  %s  %s  %dpts  (impl: %s)\n",
						issue.ID, issue.Title, issue.Priority, issue.Points, issue.ImplementerSession)
				}
			}
		}

		if jsonOutput {
			return output.JSON(result)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(focusCmd)
	rootCmd.AddCommand(unfocusCmd)
	rootCmd.AddCommand(currentCmd)

	currentCmd.Flags().Bool("json", false, "JSON output")
}
