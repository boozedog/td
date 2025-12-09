package cmd

import (
	"fmt"
	"sort"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/output"
	"github.com/spf13/cobra"
)

var blockedByCmd = &cobra.Command{
	Use:   "blocked-by [issue-id]",
	Short: "Show what issues are waiting on this issue",
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
			output.Error("%v", err)
			return err
		}

		directOnly, _ := cmd.Flags().GetBool("direct")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		// Get direct blocked issues
		blocked, err := database.GetBlockedBy(issueID)
		if err != nil {
			output.Error("failed to get blocked issues: %v", err)
			return err
		}

		result := map[string]interface{}{
			"issue":        issue,
			"direct":       blocked,
			"direct_count": len(blocked),
		}

		if !directOnly {
			// Get transitive blocked issues
			allBlocked := getTransitiveBlocked(database, issueID, make(map[string]bool))
			transitiveCount := len(allBlocked) - len(blocked)
			result["transitive_count"] = transitiveCount
			result["all"] = allBlocked
		}

		if jsonOutput {
			return output.JSON(result)
		}

		// Text output
		fmt.Printf("%s: %s %s\n", issue.ID, issue.Title, output.FormatStatus(issue.Status))

		if len(blocked) == 0 {
			fmt.Println("No issues blocked by this one")
			return nil
		}

		printBlockedTree(database, issueID, 0, make(map[string]bool), directOnly)

		directCount := len(blocked)
		if !directOnly {
			allBlocked := getTransitiveBlocked(database, issueID, make(map[string]bool))
			transitiveCount := len(allBlocked) - directCount
			fmt.Printf("\n%d issues blocked (%d direct, %d transitive)\n", len(allBlocked), directCount, transitiveCount)
		} else {
			fmt.Printf("\n%d issues directly blocked\n", directCount)
		}

		return nil
	},
}

func printBlockedTree(database *db.DB, issueID string, depth int, visited map[string]bool, directOnly bool) {
	blocked, _ := database.GetBlockedBy(issueID)

	if depth == 0 {
		fmt.Println("└── blocks:")
	}

	for i, id := range blocked {
		if visited[id] {
			continue
		}
		visited[id] = true

		issue, err := database.GetIssue(id)
		if err != nil {
			continue
		}

		prefix := "    "
		for j := 0; j < depth; j++ {
			prefix += "    "
		}

		isLast := i == len(blocked)-1
		if isLast {
			fmt.Printf("%s└── %s: %s %s\n", prefix, issue.ID, issue.Title, output.FormatStatus(issue.Status))
		} else {
			fmt.Printf("%s├── %s: %s %s\n", prefix, issue.ID, issue.Title, output.FormatStatus(issue.Status))
		}

		if !directOnly {
			printBlockedTree(database, id, depth+1, visited, directOnly)
		}
	}
}

func getTransitiveBlocked(database *db.DB, issueID string, visited map[string]bool) []string {
	if visited[issueID] {
		return nil
	}
	visited[issueID] = true

	blocked, _ := database.GetBlockedBy(issueID)
	var all []string
	all = append(all, blocked...)

	for _, id := range blocked {
		all = append(all, getTransitiveBlocked(database, id, visited)...)
	}

	return all
}

var dependsOnCmd = &cobra.Command{
	Use:   "depends-on [issue-id]",
	Short: "Show what issues this issue depends on",
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
			output.Error("%v", err)
			return err
		}

		deps, err := database.GetDependencies(issueID)
		if err != nil {
			output.Error("failed to get dependencies: %v", err)
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")

		if jsonOutput {
			result := map[string]interface{}{
				"issue":        issue,
				"dependencies": deps,
			}
			return output.JSON(result)
		}

		fmt.Printf("%s: %s %s\n", issue.ID, issue.Title, output.FormatStatus(issue.Status))

		if len(deps) == 0 {
			fmt.Println("No dependencies")
			return nil
		}

		fmt.Println("└── depends on:")
		blocking := 0
		resolved := 0

		for _, depID := range deps {
			dep, err := database.GetIssue(depID)
			if err != nil {
				continue
			}

			statusMark := ""
			if dep.Status == models.StatusClosed {
				statusMark = " ✓"
				resolved++
			} else {
				blocking++
			}

			fmt.Printf("    %s: %s %s%s\n", dep.ID, dep.Title, output.FormatStatus(dep.Status), statusMark)
		}

		fmt.Printf("\n%d blocking, %d resolved\n", blocking, resolved)

		return nil
	},
}

var criticalPathCmd = &cobra.Command{
	Use:   "critical-path",
	Short: "Show the sequence of issues that unblocks the most work",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		if limit == 0 {
			limit = 10
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")

		// Get all open/in_progress issues
		issues, err := database.ListIssues(db.ListIssuesOptions{
			Status: []models.Status{models.StatusOpen, models.StatusInProgress, models.StatusBlocked},
		})
		if err != nil {
			output.Error("failed to list issues: %v", err)
			return err
		}

		// Calculate how many issues each issue blocks
		blockCounts := make(map[string]int)
		for _, issue := range issues {
			count := len(getTransitiveBlocked(database, issue.ID, make(map[string]bool)))
			if count > 0 {
				blockCounts[issue.ID] = count
			}
		}

		// Sort by block count
		type issueScore struct {
			id    string
			score int
		}
		var scores []issueScore
		for id, count := range blockCounts {
			scores = append(scores, issueScore{id, count})
		}
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].score > scores[j].score
		})

		if jsonOutput {
			result := make([]map[string]interface{}, 0)
			for i, s := range scores {
				if i >= limit {
					break
				}
				issue, _ := database.GetIssue(s.id)
				if issue != nil {
					result = append(result, map[string]interface{}{
						"issue":         issue,
						"blocks_count":  s.score,
						"critical_rank": i + 1,
					})
				}
			}
			return output.JSON(result)
		}

		if len(scores) == 0 {
			fmt.Println("No blocking dependencies found")
			return nil
		}

		fmt.Println("CRITICAL PATH (unblocks most issues):")
		fmt.Println()

		for i, s := range scores {
			if i >= limit {
				break
			}
			issue, _ := database.GetIssue(s.id)
			if issue == nil {
				continue
			}

			fmt.Printf("%d. %s  %s  %s  %s  %dpts\n",
				i+1, issue.ID, issue.Title, output.FormatStatus(issue.Status), issue.Priority, issue.Points)
			fmt.Printf("   └─▶ unblocks %d\n", s.score)
		}

		fmt.Println()
		fmt.Println("BOTTLENECKS (blocking most issues):")

		shown := 0
		for _, s := range scores {
			if shown >= 3 {
				break
			}
			fmt.Printf("  %s: %d issues waiting\n", s.id, s.score)
			shown++
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(blockedByCmd)
	rootCmd.AddCommand(dependsOnCmd)
	rootCmd.AddCommand(criticalPathCmd)

	blockedByCmd.Flags().Bool("direct", false, "Only show direct dependencies")
	blockedByCmd.Flags().Bool("json", false, "JSON output")

	dependsOnCmd.Flags().Bool("json", false, "JSON output")

	criticalPathCmd.Flags().Int("limit", 10, "Max issues to show")
	criticalPathCmd.Flags().Bool("json", false, "JSON output")
}
