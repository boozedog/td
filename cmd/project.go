package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/output"
	"github.com/marcus/td/internal/syncclient"
	"github.com/marcus/td/internal/syncconfig"
	"github.com/spf13/cobra"
)

var syncProjectCmd = &cobra.Command{
	Use:     "sync-project",
	Aliases: []string{"sp"},
	Short:   "Manage sync projects",
	GroupID: "system",
}

var syncProjectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a remote sync project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !syncconfig.IsAuthenticated() {
			output.Error("not logged in (run: td auth login)")
			return fmt.Errorf("not authenticated")
		}

		name := args[0]
		description, _ := cmd.Flags().GetString("description")

		serverURL := syncconfig.GetServerURL()
		apiKey := syncconfig.GetAPIKey()
		client := syncclient.New(serverURL, apiKey, "")

		project, err := client.CreateProject(name, description)
		if err != nil {
			output.Error("create project: %v", err)
			return err
		}

		output.Success("Created project %s (%s)", project.Name, project.ID)
		return nil
	},
}

var syncProjectLinkCmd = &cobra.Command{
	Use:   "link <project-id>",
	Short: "Link local project to remote sync project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !syncconfig.IsAuthenticated() {
			output.Error("not logged in (run: td auth login)")
			return fmt.Errorf("not authenticated")
		}

		baseDir := getBaseDir()
		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("open database: %v", err)
			return err
		}
		defer database.Close()

		projectID := args[0]
		if err := database.SetSyncState(projectID); err != nil {
			output.Error("link project: %v", err)
			return err
		}

		output.Success("Linked to project %s", projectID)
		return nil
	},
}

var syncProjectUnlinkCmd = &cobra.Command{
	Use:   "unlink",
	Short: "Unlink local project from remote sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()
		database, err := db.Open(baseDir)
		if err != nil {
			output.Error("open database: %v", err)
			return err
		}
		defer database.Close()

		if err := database.ClearSyncState(); err != nil {
			output.Error("unlink project: %v", err)
			return err
		}

		output.Success("Unlinked from sync project")
		return nil
	},
}

var syncProjectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List remote sync projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !syncconfig.IsAuthenticated() {
			output.Error("not logged in (run: td auth login)")
			return fmt.Errorf("not authenticated")
		}

		serverURL := syncconfig.GetServerURL()
		apiKey := syncconfig.GetAPIKey()
		client := syncclient.New(serverURL, apiKey, "")

		projects, err := client.ListProjects()
		if err != nil {
			output.Error("list projects: %v", err)
			return err
		}

		if len(projects) == 0 {
			fmt.Println("No projects.")
			return nil
		}

		fmt.Printf("%-36s  %-20s  %s\n", "ID", "NAME", "CREATED")
		for _, p := range projects {
			fmt.Printf("%-36s  %-20s  %s\n", p.ID, p.Name, p.CreatedAt)
		}
		return nil
	},
}

func init() {
	syncProjectCreateCmd.Flags().String("description", "", "Project description")

	syncProjectCmd.AddCommand(syncProjectCreateCmd)
	syncProjectCmd.AddCommand(syncProjectLinkCmd)
	syncProjectCmd.AddCommand(syncProjectUnlinkCmd)
	syncProjectCmd.AddCommand(syncProjectListCmd)
	rootCmd.AddCommand(syncProjectCmd)
}
