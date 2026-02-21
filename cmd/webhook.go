package cmd

import (
	"fmt"
	"os"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/syncconfig"
	"github.com/marcus/td/internal/webhook"
	"github.com/spf13/cobra"
)

var webhookCmd = &cobra.Command{
	Use:     "webhook",
	Short:   "Manage webhook settings",
	GroupID: "system",
}

var webhookSetCmd = &cobra.Command{
	Use:   "set <url>",
	Short: "Set the webhook URL (and optional --secret)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]
		secret, _ := cmd.Flags().GetString("secret")
		global, _ := cmd.Flags().GetBool("global")

		if global {
			gcfg, err := syncconfig.LoadConfig()
			if err != nil {
				return fmt.Errorf("load global config: %w", err)
			}
			if gcfg.Webhook == nil {
				gcfg.Webhook = &models.WebhookConfig{}
			}
			gcfg.Webhook.URL = url
			if cmd.Flags().Changed("secret") {
				gcfg.Webhook.Secret = secret
			}
			if err := syncconfig.SaveConfig(gcfg); err != nil {
				return fmt.Errorf("save global config: %w", err)
			}
			fmt.Printf("Global webhook URL set: %s\n", url)
			if gcfg.Webhook.Secret != "" {
				fmt.Println("HMAC secret: configured")
			}
			return nil
		}

		baseDir := getBaseDir()
		cfg, err := config.Load(baseDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if cfg.Webhook == nil {
			cfg.Webhook = &models.WebhookConfig{}
		}
		cfg.Webhook.URL = url
		if cmd.Flags().Changed("secret") {
			cfg.Webhook.Secret = secret
		}
		if err := config.Save(baseDir, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Printf("Webhook URL set: %s\n", url)
		if cfg.Webhook.Secret != "" {
			fmt.Println("HMAC secret: configured")
		}
		return nil
	},
}

var webhookRemoveCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Remove webhook configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		global, _ := cmd.Flags().GetBool("global")

		if global {
			gcfg, err := syncconfig.LoadConfig()
			if err != nil {
				return fmt.Errorf("load global config: %w", err)
			}
			gcfg.Webhook = nil
			if err := syncconfig.SaveConfig(gcfg); err != nil {
				return fmt.Errorf("save global config: %w", err)
			}
			fmt.Println("Global webhook configuration removed.")
			return nil
		}

		baseDir := getBaseDir()
		cfg, err := config.Load(baseDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg.Webhook = nil
		if err := config.Save(baseDir, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Println("Webhook configuration removed.")
		return nil
	},
}

var webhookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current webhook configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()

		// Show effective (resolved) config.
		url := webhook.GetURL(baseDir)
		if url == "" {
			fmt.Println("Webhook: not configured")
			return nil
		}
		fmt.Printf("Webhook URL: %s\n", url)
		if webhook.GetSecret(baseDir) != "" {
			fmt.Println("HMAC secret: configured")
		} else {
			fmt.Println("HMAC secret: not set")
		}

		// Show where the URL is coming from.
		source := "env"
		if os.Getenv("TD_WEBHOOK_URL") == "" {
			cfg, err := config.Load(baseDir)
			if err == nil && cfg.Webhook != nil && cfg.Webhook.URL != "" {
				source = "project"
			} else {
				source = "global"
			}
		}
		fmt.Printf("Source: %s\n", source)
		return nil
	},
}

var webhookTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a test webhook payload (synchronous)",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir := getBaseDir()
		url := webhook.GetURL(baseDir)
		if url == "" {
			return fmt.Errorf("no webhook URL configured (use: td webhook set <url>)")
		}
		secret := webhook.GetSecret(baseDir)

		payload := webhook.BuildPayload(baseDir, nil)
		payload.Actions = []webhook.ActionPayload{
			{
				ID:         "test-ping",
				ActionType: "test",
				EntityType: "webhook",
				EntityID:   "test",
				NewData:    `{"message":"webhook test from td"}`,
				Timestamp:  payload.Timestamp,
			},
		}

		fmt.Printf("Sending test webhook to %s ... ", url)
		if err := webhook.Dispatch(url, secret, payload); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("webhook delivery failed: %w", err)
		}
		fmt.Println("OK")
		return nil
	},
}

func init() {
	webhookSetCmd.Flags().String("secret", "", "HMAC-SHA256 signing secret")
	webhookSetCmd.Flags().BoolP("global", "g", false, "Set in global config (~/.config/td/config.json)")
	webhookRemoveCmd.Flags().BoolP("global", "g", false, "Remove from global config (~/.config/td/config.json)")
	webhookCmd.AddCommand(webhookSetCmd, webhookRemoveCmd, webhookStatusCmd, webhookTestCmd)
	rootCmd.AddCommand(webhookCmd)
}
