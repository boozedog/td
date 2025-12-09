package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/marcus/td/internal/models"
)

const configFile = ".todos/config.json"

// Load reads the config from disk
func Load(baseDir string) (*models.Config, error) {
	configPath := filepath.Join(baseDir, configFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.Config{}, nil
		}
		return nil, err
	}

	var cfg models.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes the config to disk
func Save(baseDir string, cfg *models.Config) error {
	configPath := filepath.Join(baseDir, configFile)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// SetFocus sets the focused issue ID
func SetFocus(baseDir string, issueID string) error {
	cfg, err := Load(baseDir)
	if err != nil {
		return err
	}

	cfg.FocusedIssueID = issueID
	return Save(baseDir, cfg)
}

// ClearFocus clears the focused issue
func ClearFocus(baseDir string) error {
	return SetFocus(baseDir, "")
}

// GetFocus returns the focused issue ID
func GetFocus(baseDir string) (string, error) {
	cfg, err := Load(baseDir)
	if err != nil {
		return "", err
	}
	return cfg.FocusedIssueID, nil
}

// SetActiveWorkSession sets the active work session ID
func SetActiveWorkSession(baseDir string, wsID string) error {
	cfg, err := Load(baseDir)
	if err != nil {
		return err
	}

	cfg.ActiveWorkSession = wsID
	return Save(baseDir, cfg)
}

// GetActiveWorkSession returns the active work session ID
func GetActiveWorkSession(baseDir string) (string, error) {
	cfg, err := Load(baseDir)
	if err != nil {
		return "", err
	}
	return cfg.ActiveWorkSession, nil
}

// ClearActiveWorkSession clears the active work session
func ClearActiveWorkSession(baseDir string) error {
	return SetActiveWorkSession(baseDir, "")
}
