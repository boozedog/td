package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	sessionFile = ".todos/session"
	sessionPrefix = "ses_"
)

// Session represents the current terminal session
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// generateID creates a new random session ID
func generateID() (string, error) {
	bytes := make([]byte, 3) // 6 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return sessionPrefix + hex.EncodeToString(bytes), nil
}

// GetOrCreate returns the current session, creating one if necessary
func GetOrCreate(baseDir string) (*Session, error) {
	sessionPath := filepath.Join(baseDir, sessionFile)

	// Check if session file exists
	data, err := os.ReadFile(sessionPath)
	if err == nil {
		// Parse existing session
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) >= 2 {
			sess := &Session{
				ID: strings.TrimSpace(lines[0]),
			}
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
				sess.StartedAt = t
			}
			if len(lines) >= 3 {
				sess.Name = strings.TrimSpace(lines[2])
			}
			return sess, nil
		}
	}

	// Create new session
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:        id,
		StartedAt: time.Now(),
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	// Write session file
	if err := Save(baseDir, sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// Save writes the session to disk
func Save(baseDir string, sess *Session) error {
	sessionPath := filepath.Join(baseDir, sessionFile)

	content := fmt.Sprintf("%s\n%s\n%s\n", sess.ID, sess.StartedAt.Format(time.RFC3339), sess.Name)
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// SetName sets the session name
func SetName(baseDir string, name string) (*Session, error) {
	sess, err := GetOrCreate(baseDir)
	if err != nil {
		return nil, err
	}

	sess.Name = name
	if err := Save(baseDir, sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// Get returns the current session without creating one
func Get(baseDir string) (*Session, error) {
	sessionPath := filepath.Join(baseDir, sessionFile)

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("session not found: run 'td init' first")
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid session file")
	}

	sess := &Session{
		ID: strings.TrimSpace(lines[0]),
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
		sess.StartedAt = t
	}
	if len(lines) >= 3 {
		sess.Name = strings.TrimSpace(lines[2])
	}

	return sess, nil
}
