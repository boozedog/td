package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/output"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Full-text search across issues",
	Long:  `Search title, description, logs, and handoff content.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		query := args[0]

		opts := db.ListIssuesOptions{
			Search: query,
		}

		// Parse status filter
		if statusStr, _ := cmd.Flags().GetStringArray("status"); len(statusStr) > 0 {
			for _, s := range statusStr {
				opts.Status = append(opts.Status, models.Status(s))
			}
		}

		issues, err := database.SearchIssues(query, opts)
		if err != nil {
			output.Error("search failed: %v", err)
			return err
		}

		for _, issue := range issues {
			fmt.Println(output.FormatIssueShort(&issue))
		}

		if len(issues) == 0 {
			fmt.Printf("No issues matching '%s'\n", query)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().StringArrayP("status", "s", nil, "Filter by status")
}
