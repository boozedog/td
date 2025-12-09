package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/output"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show [issue-id]",
	Short: "Display full details of an issue",
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

		issue, err := database.GetIssue(issueID)
		if err != nil {
			if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
				output.JSONError("not_found", err.Error())
			} else {
				output.Error("%v", err)
			}
			return err
		}

		// Get logs and handoff
		logs, _ := database.GetLogs(issueID, 0)
		handoff, _ := database.GetLatestHandoff(issueID)

		// Check output format
		if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
			result := map[string]interface{}{
				"id":                  issue.ID,
				"title":               issue.Title,
				"description":         issue.Description,
				"status":              issue.Status,
				"type":                issue.Type,
				"priority":            issue.Priority,
				"points":              issue.Points,
				"labels":              issue.Labels,
				"parent_id":           issue.ParentID,
				"acceptance":          issue.Acceptance,
				"implementer_session": issue.ImplementerSession,
				"reviewer_session":    issue.ReviewerSession,
				"created_at":          issue.CreatedAt,
				"updated_at":          issue.UpdatedAt,
			}
			if issue.ClosedAt != nil {
				result["closed_at"] = issue.ClosedAt
			}
			if handoff != nil {
				result["handoff"] = map[string]interface{}{
					"timestamp": handoff.Timestamp,
					"session":   handoff.SessionID,
					"done":      handoff.Done,
					"remaining": handoff.Remaining,
					"decisions": handoff.Decisions,
					"uncertain": handoff.Uncertain,
				}
			}
			if len(logs) > 0 {
				logEntries := make([]map[string]interface{}, len(logs))
				for i, log := range logs {
					logEntries[i] = map[string]interface{}{
						"timestamp": log.Timestamp,
						"message":   log.Message,
						"type":      log.Type,
					}
				}
				result["logs"] = logEntries
			}
			return output.JSON(result)
		}

		if short, _ := cmd.Flags().GetBool("short"); short {
			fmt.Println(output.FormatIssueShort(issue))
			return nil
		}

		// Long format (default)
		fmt.Print(output.FormatIssueLong(issue, logs, handoff))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)

	showCmd.Flags().Bool("long", false, "Detailed multi-line output (default)")
	showCmd.Flags().Bool("short", false, "Compact summary")
	showCmd.Flags().Bool("json", false, "Machine-readable JSON")
}
