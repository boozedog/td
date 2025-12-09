package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/output"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link [issue-id] [file-pattern]",
	Short: "Link files to an issue",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		issueID := args[0]
		pattern := args[1]

		// Verify issue exists
		_, err = database.GetIssue(issueID)
		if err != nil {
			output.Error("%v", err)
			return err
		}

		// Get role
		roleStr, _ := cmd.Flags().GetString("role")
		role := models.FileRoleImplementation
		if roleStr != "" {
			role = models.FileRole(roleStr)
		}

		// Find matching files
		matches, err := filepath.Glob(pattern)
		if err != nil {
			output.Error("invalid pattern: %v", err)
			return err
		}

		if len(matches) == 0 {
			// Try as a literal path
			if _, err := os.Stat(pattern); err == nil {
				matches = []string{pattern}
			} else {
				output.Error("no files matching pattern: %s", pattern)
				return fmt.Errorf("no matches")
			}
		}

		// Handle directories
		recursive, _ := cmd.Flags().GetBool("recursive")
		var allFiles []string

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}

			if info.IsDir() {
				if recursive {
					filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return nil
						}
						if !info.IsDir() {
							allFiles = append(allFiles, path)
						}
						return nil
					})
				} else {
					// Just files in the directory
					entries, _ := os.ReadDir(match)
					for _, entry := range entries {
						if !entry.IsDir() {
							allFiles = append(allFiles, filepath.Join(match, entry.Name()))
						}
					}
				}
			} else {
				allFiles = append(allFiles, match)
			}
		}

		// Link each file
		count := 0
		for _, file := range allFiles {
			// Get absolute path
			absPath, _ := filepath.Abs(file)

			// For now, we don't compute SHA of the file
			if err := database.LinkFile(issueID, absPath, role, ""); err != nil {
				output.Warning("failed to link %s: %v", file, err)
				continue
			}
			count++
		}

		if count == 1 {
			fmt.Printf("LINKED 1 file to %s\n", issueID)
		} else {
			fmt.Printf("LINKED %d files to %s\n", count, issueID)
		}

		return nil
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink [issue-id] [file-pattern]",
	Short: "Remove file associations",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("%v", err)
			return err
		}
		defer database.Close()

		issueID := args[0]
		pattern := args[1]

		// Get linked files
		files, err := database.GetLinkedFiles(issueID)
		if err != nil {
			output.Error("failed to get linked files: %v", err)
			return err
		}

		count := 0
		for _, file := range files {
			matched, _ := filepath.Match(pattern, file.FilePath)
			if matched || file.FilePath == pattern {
				if err := database.UnlinkFile(issueID, file.FilePath); err != nil {
					output.Warning("failed to unlink %s: %v", file.FilePath, err)
					continue
				}
				count++
			}
		}

		if count == 1 {
			fmt.Printf("UNLINKED 1 file from %s\n", issueID)
		} else {
			fmt.Printf("UNLINKED %d files from %s\n", count, issueID)
		}

		return nil
	},
}

var filesCmd = &cobra.Command{
	Use:   "files [issue-id]",
	Short: "List linked files with change status",
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

		files, err := database.GetLinkedFiles(issueID)
		if err != nil {
			output.Error("failed to get linked files: %v", err)
			return err
		}

		if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
			return output.JSON(files)
		}

		fmt.Printf("%s: %s\n", issue.ID, issue.Title)

		// Get start snapshot
		startSnapshot, _ := database.GetStartSnapshot(issueID)
		if startSnapshot != nil {
			fmt.Printf("Started: %s (%s)\n", startSnapshot.CommitSHA[:7], output.FormatTimeAgo(startSnapshot.Timestamp))
		}
		fmt.Println()

		// Group by role
		byRole := make(map[models.FileRole][]models.IssueFile)
		for _, f := range files {
			byRole[f.Role] = append(byRole[f.Role], f)
		}

		roles := []models.FileRole{
			models.FileRoleImplementation,
			models.FileRoleTest,
			models.FileRoleReference,
			models.FileRoleConfig,
		}

		changedOnly, _ := cmd.Flags().GetBool("changed")

		for _, role := range roles {
			roleFiles := byRole[role]
			if len(roleFiles) == 0 {
				continue
			}

			fmt.Printf("%s:\n", string(role))
			for _, f := range roleFiles {
				// Check file status
				status := "[unchanged]"
				_, err := os.Stat(f.FilePath)
				if os.IsNotExist(err) {
					status = "[deleted]"
				} else if err == nil {
					// Could check if modified here
					status = "[exists]"
				}

				if changedOnly && status == "[unchanged]" {
					continue
				}

				// Use relative path if possible
				displayPath := f.FilePath
				if rel, err := filepath.Rel(baseDir, f.FilePath); err == nil {
					displayPath = rel
				}

				fmt.Printf("  %-40s %s\n", displayPath, status)
			}
			fmt.Println()
		}

		if len(files) == 0 {
			fmt.Println("No linked files")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
	rootCmd.AddCommand(filesCmd)

	linkCmd.Flags().String("role", "implementation", "File role: implementation, test, reference, config")
	linkCmd.Flags().Bool("recursive", true, "Include subdirectories")

	filesCmd.Flags().Bool("json", false, "JSON output")
	filesCmd.Flags().Bool("changed", false, "Only show changed files")
}
