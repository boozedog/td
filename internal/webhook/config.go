// Package webhook handles webhook configuration and HTTP dispatch.
package webhook

import (
	"os"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/syncconfig"
)

// GetURL returns the webhook URL for the project.
// Priority: TD_WEBHOOK_URL env > project-local config > global config.
func GetURL(baseDir string) string {
	if v := os.Getenv("TD_WEBHOOK_URL"); v != "" {
		return v
	}
	cfg, err := config.Load(baseDir)
	if err == nil && cfg.Webhook != nil && cfg.Webhook.URL != "" {
		return cfg.Webhook.URL
	}
	gcfg, err := syncconfig.LoadConfig()
	if err == nil && gcfg.Webhook != nil {
		return gcfg.Webhook.URL
	}
	return ""
}

// GetSecret returns the webhook HMAC secret.
// Priority: TD_WEBHOOK_SECRET env > project-local config > global config.
func GetSecret(baseDir string) string {
	if v := os.Getenv("TD_WEBHOOK_SECRET"); v != "" {
		return v
	}
	cfg, err := config.Load(baseDir)
	if err == nil && cfg.Webhook != nil && cfg.Webhook.Secret != "" {
		return cfg.Webhook.Secret
	}
	gcfg, err := syncconfig.LoadConfig()
	if err == nil && gcfg.Webhook != nil {
		return gcfg.Webhook.Secret
	}
	return ""
}

// IsEnabled returns true if a webhook URL is configured.
func IsEnabled(baseDir string) bool {
	return GetURL(baseDir) != ""
}
