package webhook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcus/td/internal/models"
)

// setupProjectConfig writes a project-local .todos/config.json in a temp dir
// and returns the base dir path.
func setupProjectConfig(t *testing.T, wh *models.WebhookConfig) string {
	t.Helper()
	dir := t.TempDir()
	todosDir := filepath.Join(dir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := models.Config{Webhook: wh}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(todosDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// setupGlobalConfig points HOME at a temp dir containing ~/.config/td/config.json
// with the given webhook config. Returns a cleanup function that restores HOME.
func setupGlobalConfig(t *testing.T, wh *models.WebhookConfig) {
	t.Helper()
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "td")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	type globalConfig struct {
		Webhook *models.WebhookConfig `json:"webhook,omitempty"`
	}
	data, err := json.Marshal(globalConfig{Webhook: wh})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
}

// emptyProjectDir returns a temp dir with no .todos/config.json.
func emptyProjectDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestGetURL_Priority(t *testing.T) {
	tests := []struct {
		name      string
		envURL    string
		localURL  string
		globalURL string
		want      string
	}{
		{
			name: "nothing configured",
			want: "",
		},
		{
			name:      "global only",
			globalURL: "https://global.example.com/hook",
			want:      "https://global.example.com/hook",
		},
		{
			name:     "local only",
			localURL: "https://local.example.com/hook",
			want:     "https://local.example.com/hook",
		},
		{
			name:   "env only",
			envURL: "https://env.example.com/hook",
			want:   "https://env.example.com/hook",
		},
		{
			name:      "local overrides global",
			localURL:  "https://local.example.com/hook",
			globalURL: "https://global.example.com/hook",
			want:      "https://local.example.com/hook",
		},
		{
			name:      "env overrides both",
			envURL:    "https://env.example.com/hook",
			localURL:  "https://local.example.com/hook",
			globalURL: "https://global.example.com/hook",
			want:      "https://env.example.com/hook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Isolate HOME so global config doesn't leak between tests.
			if tt.globalURL != "" {
				setupGlobalConfig(t, &models.WebhookConfig{URL: tt.globalURL})
			} else {
				// Point HOME at empty dir so no global config is found.
				t.Setenv("HOME", t.TempDir())
			}

			if tt.envURL != "" {
				t.Setenv("TD_WEBHOOK_URL", tt.envURL)
			} else {
				t.Setenv("TD_WEBHOOK_URL", "")
			}

			var baseDir string
			if tt.localURL != "" {
				baseDir = setupProjectConfig(t, &models.WebhookConfig{URL: tt.localURL})
			} else {
				baseDir = emptyProjectDir(t)
			}

			got := GetURL(baseDir)
			if got != tt.want {
				t.Errorf("GetURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSecret_Priority(t *testing.T) {
	tests := []struct {
		name         string
		envSecret    string
		localSecret  string
		globalSecret string
		want         string
	}{
		{
			name: "nothing configured",
			want: "",
		},
		{
			name:         "global only",
			globalSecret: "global-secret",
			want:         "global-secret",
		},
		{
			name:        "local only",
			localSecret: "local-secret",
			want:        "local-secret",
		},
		{
			name:      "env only",
			envSecret: "env-secret",
			want:      "env-secret",
		},
		{
			name:         "local overrides global",
			localSecret:  "local-secret",
			globalSecret: "global-secret",
			want:         "local-secret",
		},
		{
			name:         "env overrides both",
			envSecret:    "env-secret",
			localSecret:  "local-secret",
			globalSecret: "global-secret",
			want:         "env-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.globalSecret != "" {
				setupGlobalConfig(t, &models.WebhookConfig{Secret: tt.globalSecret})
			} else {
				t.Setenv("HOME", t.TempDir())
			}

			if tt.envSecret != "" {
				t.Setenv("TD_WEBHOOK_SECRET", tt.envSecret)
			} else {
				t.Setenv("TD_WEBHOOK_SECRET", "")
			}

			var baseDir string
			if tt.localSecret != "" {
				baseDir = setupProjectConfig(t, &models.WebhookConfig{Secret: tt.localSecret})
			} else {
				baseDir = emptyProjectDir(t)
			}

			got := GetSecret(baseDir)
			if got != tt.want {
				t.Errorf("GetSecret() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsEnabled(t *testing.T) {
	t.Setenv("TD_WEBHOOK_URL", "")
	t.Setenv("HOME", t.TempDir())

	if IsEnabled(emptyProjectDir(t)) {
		t.Error("IsEnabled should be false with no config")
	}

	t.Setenv("TD_WEBHOOK_URL", "https://example.com/hook")
	if !IsEnabled(emptyProjectDir(t)) {
		t.Error("IsEnabled should be true with env URL set")
	}
}
