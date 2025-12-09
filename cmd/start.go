package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/git"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/output"
	"github.com/marcus/td/internal/session"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [issue-id]",
	Short: "Begin work on an issue",
	Long:  `Records current session as implementer and captures git state.`,
	Args:  cobra.ExactArgs(1),
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

		issueID := args[0]
		issue, err := database.GetIssue(issueID)
		if err != nil {
			output.Error("%v", err)
			return err
		}

		// Check if blocked
		if issue.Status == models.StatusBlocked {
			force, _ := cmd.Flags().GetBool("force")
			if !force {
				output.Error("cannot start blocked issue: %s (use --force to override)", issueID)
				return fmt.Errorf("blocked issue")
			}
		}

		// Capture previous state for undo
		prevData, _ := json.Marshal(issue)

		// Update issue
		issue.Status = models.StatusInProgress
		issue.ImplementerSession = sess.ID

		if err := database.UpdateIssue(issue); err != nil {
			output.Error("failed to update issue: %v", err)
			return err
		}

		// Log action for undo
		newData, _ := json.Marshal(issue)
		database.LogAction(&models.ActionLog{
			SessionID:    sess.ID,
			ActionType:   models.ActionStart,
			EntityType:   "issue",
			EntityID:     issueID,
			PreviousData: string(prevData),
			NewData:      string(newData),
		})

		// Set focus
		config.SetFocus(baseDir, issueID)

		// Capture git state
		gitState, gitErr := git.GetState()

		// Log the start
		reason, _ := cmd.Flags().GetString("reason")
		logMsg := "Started work"
		if reason != "" {
			logMsg = reason
		}

		database.AddLog(&models.Log{
			IssueID:   issueID,
			SessionID: sess.ID,
			Message:   logMsg,
			Type:      models.LogTypeProgress,
		})

		// Record git snapshot
		if gitErr == nil {
			database.AddGitSnapshot(&models.GitSnapshot{
				IssueID:    issueID,
				Event:      "start",
				CommitSHA:  gitState.CommitSHA,
				Branch:     gitState.Branch,
				DirtyFiles: gitState.DirtyFiles,
			})
		}

		// Output
		fmt.Printf("STARTED %s (session: %s)\n", issueID, sess.ID)

		if gitErr == nil {
			stateStr := "clean"
			if !gitState.IsClean {
				stateStr = fmt.Sprintf("%d modified, %d untracked", gitState.Modified, gitState.Untracked)
			}
			fmt.Printf("Git: %s (%s) %s\n", gitState.CommitSHA[:7], gitState.Branch, stateStr)

			if !gitState.IsClean {
				output.Warning("Starting with uncommitted changes")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().String("reason", "", "Reason for starting work")
	startCmd.Flags().Bool("force", false, "Force start even if blocked")
}
