package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/marcus/td/internal/git"
)

const (
	sessionFile    = ".todos/session"         // legacy path
	sessionsDir    = ".todos/sessions"        // new branch-scoped directory
	sessionPrefix  = "ses_"
	defaultBranch  = "default"                // used when git not available
)

// Session represents the current terminal session
type Session struct {
	ID                string    `json:"id"`
	Name              string    `json:"name,omitempty"`
	Branch            string    `json:"branch,omitempty"`            // git branch for session scoping
	AgentType         string    `json:"agent_type,omitempty"`        // agent type (claude-code, cursor, terminal, etc.)
	AgentPID          int       `json:"agent_pid,omitempty"`         // stable parent agent process ID
	ContextID         string    `json:"context_id,omitempty"`        // audit only, not used for matching
	PreviousSessionID string    `json:"previous_session_id,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	LastActivity      time.Time `json:"last_activity,omitempty"`     // heartbeat for session liveness
	IsNew             bool      `json:"-"` // True if session was just created (not persisted)
}

// Display returns the session ID with name if set: "ses_abc123 (my-name)" or just "ses_abc123"
func (s *Session) Display() string {
	if s.Name != "" {
		return fmt.Sprintf("%s (%s)", s.ID, s.Name)
	}
	return s.ID
}

// DisplayWithAgent returns session info including agent: "ses_abc123 [claude-code]" or with name
func (s *Session) DisplayWithAgent() string {
	base := s.ID
	if s.Name != "" {
		base = fmt.Sprintf("%s (%s)", s.ID, s.Name)
	}
	if s.AgentType != "" {
		return fmt.Sprintf("%s [%s]", base, s.AgentType)
	}
	return base
}

// FormatSessionID formats a session ID with optional name lookup.
// Use this when you only have a session ID string and need to display it.
// If the session has a name, returns "ses_xxx (name)", otherwise just "ses_xxx".
func FormatSessionID(baseDir, sessionID string) string {
	// Try to look up the session to get its name
	sess, err := Get(baseDir)
	if err == nil && sess.ID == sessionID && sess.Name != "" {
		return fmt.Sprintf("%s (%s)", sessionID, sess.Name)
	}
	return sessionID
}

// generateID creates a new random session ID
func generateID() (string, error) {
	bytes := make([]byte, 3) // 6 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return sessionPrefix + hex.EncodeToString(bytes), nil
}

// sanitizeBranchName converts a git branch name to a safe filename
func sanitizeBranchName(branch string) string {
	if branch == "" {
		return defaultBranch
	}
	// Replace path separators and problematic characters
	result := strings.ReplaceAll(branch, "/", "_")
	result = strings.ReplaceAll(result, "\\", "_")
	result = strings.ReplaceAll(result, ":", "_")
	result = strings.ReplaceAll(result, "*", "_")
	result = strings.ReplaceAll(result, "?", "_")
	result = strings.ReplaceAll(result, "\"", "_")
	result = strings.ReplaceAll(result, "<", "_")
	result = strings.ReplaceAll(result, ">", "_")
	result = strings.ReplaceAll(result, "|", "_")
	return result
}

// sessionPathForBranch returns the path to the session file for a given branch (legacy, branch-only)
func sessionPathForBranch(baseDir, branch string) string {
	return filepath.Join(baseDir, sessionsDir, sanitizeBranchName(branch)+".json")
}

// sessionPathForAgent returns the path to the session file for a given branch and agent
// Path format: .todos/sessions/<branch>/<agent_pid>.json
func sessionPathForAgent(baseDir, branch string, fp AgentFingerprint) string {
	branchDir := filepath.Join(baseDir, sessionsDir, sanitizeBranchName(branch))
	return filepath.Join(branchDir, fp.String()+".json")
}

// getCurrentBranch returns the current git branch, or "default" if not in a repo
func getCurrentBranch() string {
	state, err := git.GetState()
	if err != nil {
		return defaultBranch
	}
	branch := state.Branch
	if branch == "" || branch == "HEAD" {
		// Detached HEAD - use short commit SHA
		if len(state.CommitSHA) >= 8 {
			return "detached-" + state.CommitSHA[:8]
		}
		return defaultBranch
	}
	return branch
}

// getContextID generates a unique identifier for the current execution context.
// This detects when a new terminal/AI session has started.
func getContextID() string {
	// Priority 1: Explicit AI agent session IDs
	// Set TD_SESSION_ID to force a specific session context (most reliable)
	if val := os.Getenv("TD_SESSION_ID"); val != "" {
		return "explicit:" + val
	}

	// Priority 2: AI agent session IDs (Claude Code, etc.)
	for _, envVar := range []string{
		"CLAUDE_CODE_SSE_PORT", // Claude Code SSE port (unique per session)
		"CLAUDE_SESSION_ID",    // Claude Code session (if set)
		"ANTHROPIC_SESSION_ID", // Generic Anthropic
		"AI_SESSION_ID",        // Generic AI
		"CURSOR_SESSION_ID",    // Cursor IDE
		"COPILOT_SESSION_ID",   // GitHub Copilot
	} {
		if val := os.Getenv(envVar); val != "" {
			return "ai:" + val
		}
	}

	// Priority 3: Terminal session IDs (stable across command runs)
	for _, envVar := range []string{
		"TERM_SESSION_ID",       // iTerm2
		"WINDOWID",              // X11 window ID
		"TMUX_PANE",             // tmux pane
		"STY",                   // screen session
		"KONSOLE_DBUS_SESSION",  // KDE Konsole
		"GNOME_TERMINAL_SCREEN", // GNOME Terminal
		"SSH_TTY",               // stable-ish per SSH terminal
	} {
		if val := os.Getenv(envVar); val != "" {
			return "term:" + envVar + "=" + val
		}
	}

	// Priority 4: Best-effort process + tty fingerprint.
	// os.Getppid() should be stable across commands in the same shell, and differ across terminals.
	ppid := os.Getppid()

	// Prefer a tty path if available. This helps disambiguate scenarios where ppid alone is too coarse.
	tty := ""
	if link, err := os.Readlink("/dev/fd/0"); err == nil {
		tty = link
	}

	if tty != "" {
		return fmt.Sprintf("proc:ppid=%d tty=%s", ppid, tty)
	}
	if shlvl := os.Getenv("SHLVL"); shlvl != "" {
		return fmt.Sprintf("proc:ppid=%d shlvl=%s", ppid, shlvl)
	}
	return fmt.Sprintf("proc:ppid=%d", ppid)
}

// GetOrCreate returns the current session for the current git branch and agent.
// Sessions are scoped by branch + agent fingerprint - same agent on same branch = same session.
// Creates a new session if none exists for this branch/agent combination.
func GetOrCreate(baseDir string) (*Session, error) {
	branch := getCurrentBranch()
	fp := GetAgentFingerprint()
	agentPath := sessionPathForAgent(baseDir, branch, fp)

	// Ensure project is initialized. Avoid creating .todos/ as a side effect.
	todosDir := filepath.Join(baseDir, ".todos")
	if _, err := os.Stat(todosDir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: run 'td init' first")
		}
		return nil, fmt.Errorf("stat %s: %w", todosDir, err)
	}

	// Check if agent-scoped session file exists
	data, err := os.ReadFile(agentPath)
	if err == nil {
		var sess Session
		if err := json.Unmarshal(data, &sess); err == nil {
			// Session exists for this agent - reuse it, update heartbeat
			sess.IsNew = false
			sess.LastActivity = time.Now()
			// Save to update heartbeat
			if saveErr := saveToBranchPath(agentPath, &sess); saveErr != nil {
				// Non-fatal, just log
			}
			return &sess, nil
		}
	}

	// Check for legacy branch-scoped session to migrate
	branchPath := sessionPathForBranch(baseDir, branch)
	if branchData, branchErr := os.ReadFile(branchPath); branchErr == nil {
		if sess, migrateErr := migrateBranchSession(branchData, branch, fp, agentPath); migrateErr == nil {
			os.Remove(branchPath) // Clean up old file after successful migration
			return sess, nil
		}
	}

	// Check for legacy .todos/session file to migrate
	legacyPath := filepath.Join(baseDir, sessionFile)
	if legacyData, legacyErr := os.ReadFile(legacyPath); legacyErr == nil {
		if sess, migrateErr := migrateLegacyToAgentSession(legacyData, branch, fp, agentPath); migrateErr == nil {
			os.Remove(legacyPath) // Clean up legacy file after successful migration
			return sess, nil
		}
	}

	// No session found - create new one for this branch/agent
	return createAgentSession(baseDir, branch, fp, "")
}

// migrateLegacySession attempts to migrate a legacy session file to branch-scoped format
func migrateLegacySession(data []byte, branch, branchPath string) (*Session, error) {
	var sess Session

	// Try JSON format first
	if err := json.Unmarshal(data, &sess); err != nil {
		// Try legacy line-based format
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) < 2 {
			return nil, fmt.Errorf("invalid legacy session format")
		}
		sess.ID = strings.TrimSpace(lines[0])
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
			sess.StartedAt = t
		}
		if len(lines) >= 3 {
			sess.ContextID = strings.TrimSpace(lines[2])
		}
		if len(lines) >= 4 {
			sess.Name = strings.TrimSpace(lines[3])
		}
		if len(lines) >= 5 {
			sess.PreviousSessionID = strings.TrimSpace(lines[4])
		}
	}

	// Update for branch-scoped format
	sess.Branch = branch
	sess.LastActivity = time.Now()
	sess.IsNew = false

	// Ensure sessions directory exists
	if err := os.MkdirAll(filepath.Dir(branchPath), 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	// Save to branch path
	if err := saveToBranchPath(branchPath, &sess); err != nil {
		return nil, err
	}

	return &sess, nil
}

// migrateBranchSession migrates a branch-scoped session to agent-scoped format
func migrateBranchSession(data []byte, branch string, fp AgentFingerprint, agentPath string) (*Session, error) {
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal branch session: %w", err)
	}

	// Update for agent-scoped format
	sess.Branch = branch
	sess.AgentType = string(fp.Type)
	sess.AgentPID = fp.PID
	sess.LastActivity = time.Now()
	sess.IsNew = false

	// Ensure branch subdirectory exists
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return nil, fmt.Errorf("create agent sessions dir: %w", err)
	}

	// Save to agent path
	if err := saveToBranchPath(agentPath, &sess); err != nil {
		return nil, err
	}

	return &sess, nil
}

// migrateLegacyToAgentSession migrates a legacy .todos/session file to agent-scoped format
func migrateLegacyToAgentSession(data []byte, branch string, fp AgentFingerprint, agentPath string) (*Session, error) {
	var sess Session

	// Try JSON format first
	if err := json.Unmarshal(data, &sess); err != nil {
		// Try legacy line-based format
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) < 2 {
			return nil, fmt.Errorf("invalid legacy session format")
		}
		sess.ID = strings.TrimSpace(lines[0])
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
			sess.StartedAt = t
		}
		if len(lines) >= 3 {
			sess.ContextID = strings.TrimSpace(lines[2])
		}
		if len(lines) >= 4 {
			sess.Name = strings.TrimSpace(lines[3])
		}
		if len(lines) >= 5 {
			sess.PreviousSessionID = strings.TrimSpace(lines[4])
		}
	}

	// Update for agent-scoped format
	sess.Branch = branch
	sess.AgentType = string(fp.Type)
	sess.AgentPID = fp.PID
	sess.LastActivity = time.Now()
	sess.IsNew = false

	// Ensure branch subdirectory exists
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return nil, fmt.Errorf("create agent sessions dir: %w", err)
	}

	// Save to agent path
	if err := saveToBranchPath(agentPath, &sess); err != nil {
		return nil, err
	}

	return &sess, nil
}

// createAgentSession creates a new session for the given branch and agent
func createAgentSession(baseDir, branch string, fp AgentFingerprint, previousID string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:                id,
		Branch:            branch,
		AgentType:         string(fp.Type),
		AgentPID:          fp.PID,
		ContextID:         getContextID(), // for audit only
		PreviousSessionID: previousID,
		StartedAt:         time.Now(),
		LastActivity:      time.Now(),
		IsNew:             true,
	}

	agentPath := sessionPathForAgent(baseDir, branch, fp)

	// Ensure branch subdirectory exists
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return nil, fmt.Errorf("create agent sessions dir: %w", err)
	}

	// Save to agent path
	if err := saveToBranchPath(agentPath, sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// createBranchSession creates a new session for the given branch
func createBranchSession(baseDir, branch, previousID string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:                id,
		Branch:            branch,
		ContextID:         getContextID(), // for audit only
		PreviousSessionID: previousID,
		StartedAt:         time.Now(),
		LastActivity:      time.Now(),
		IsNew:             true,
	}

	branchPath := sessionPathForBranch(baseDir, branch)

	// Ensure sessions directory exists
	if err := os.MkdirAll(filepath.Dir(branchPath), 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	// Save to branch path
	if err := saveToBranchPath(branchPath, sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// saveToBranchPath saves a session to a specific path
func saveToBranchPath(path string, sess *Session) error {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

// Save writes the session to disk as JSON (agent-scoped)
func Save(baseDir string, sess *Session) error {
	// Ensure branch is set
	if sess.Branch == "" {
		sess.Branch = getCurrentBranch()
	}

	// Build fingerprint from session fields or detect fresh
	var fp AgentFingerprint
	if sess.AgentType != "" {
		fp = AgentFingerprint{Type: AgentType(sess.AgentType), PID: sess.AgentPID}
	} else {
		fp = GetAgentFingerprint()
		sess.AgentType = string(fp.Type)
		sess.AgentPID = fp.PID
	}

	agentPath := sessionPathForAgent(baseDir, sess.Branch, fp)

	// Ensure branch subdirectory exists
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return fmt.Errorf("create agent sessions dir: %w", err)
	}

	return saveToBranchPath(agentPath, sess)
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

// Get returns the current session without creating one (agent-scoped)
func Get(baseDir string) (*Session, error) {
	branch := getCurrentBranch()
	fp := GetAgentFingerprint()
	agentPath := sessionPathForAgent(baseDir, branch, fp)

	// Try agent-scoped session first
	if data, err := os.ReadFile(agentPath); err == nil {
		var sess Session
		if err := json.Unmarshal(data, &sess); err == nil {
			return &sess, nil
		}
	}

	// Fallback to branch-scoped session
	branchPath := sessionPathForBranch(baseDir, branch)
	if data, err := os.ReadFile(branchPath); err == nil {
		var sess Session
		if err := json.Unmarshal(data, &sess); err == nil {
			return &sess, nil
		}
	}

	// Fallback to legacy session file
	legacyPath := filepath.Join(baseDir, sessionFile)
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return nil, fmt.Errorf("session not found: run 'td init' first")
	}

	// Try JSON format first
	var sess Session
	if err := json.Unmarshal(data, &sess); err == nil {
		return &sess, nil
	}

	// Fallback: legacy line-based format
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid session file")
	}

	sess = Session{
		ID: strings.TrimSpace(lines[0]),
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1])); err == nil {
		sess.StartedAt = t
	}
	if len(lines) >= 3 {
		sess.ContextID = strings.TrimSpace(lines[2])
	}
	if len(lines) >= 4 {
		sess.Name = strings.TrimSpace(lines[3])
	}
	if len(lines) >= 5 {
		sess.PreviousSessionID = strings.TrimSpace(lines[4])
	}

	return &sess, nil
}

// GetWithContextCheck returns the current session and checks if context changed.
// If context changed, creates a new session automatically.
func GetWithContextCheck(baseDir string) (*Session, error) {
	return GetOrCreate(baseDir)
}

// ForceNewSession creates a new session on the current branch/agent, regardless of existing session
func ForceNewSession(baseDir string) (*Session, error) {
	branch := getCurrentBranch()
	fp := GetAgentFingerprint()

	// Ensure project is initialized. Avoid creating .todos/ as a side effect.
	todosDir := filepath.Join(baseDir, ".todos")
	if _, err := os.Stat(todosDir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: run 'td init' first")
		}
		return nil, fmt.Errorf("stat %s: %w", todosDir, err)
	}

	// Get previous session ID if exists
	var previousID string
	if existing, err := Get(baseDir); err == nil {
		previousID = existing.ID
	}

	return createAgentSession(baseDir, branch, fp, previousID)
}

// ParseDuration parses human-readable duration strings
func ParseDuration(s string) (time.Duration, error) {
	// Try standard duration first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle day format
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("invalid duration: %s", s)
}

// ListSessions returns all sessions (agent-scoped and legacy branch-scoped)
func ListSessions(baseDir string) ([]Session, error) {
	sessionsPath := filepath.Join(baseDir, sessionsDir)

	entries, err := os.ReadDir(sessionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No sessions directory yet
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var sessions []Session
	for _, entry := range entries {
		entryPath := filepath.Join(sessionsPath, entry.Name())

		if entry.IsDir() {
			// Agent-scoped sessions in branch subdirectory
			subEntries, err := os.ReadDir(entryPath)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if subEntry.IsDir() || !strings.HasSuffix(subEntry.Name(), ".json") {
					continue
				}
				path := filepath.Join(entryPath, subEntry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				var sess Session
				if err := json.Unmarshal(data, &sess); err != nil {
					continue
				}
				sessions = append(sessions, sess)
			}
		} else if strings.HasSuffix(entry.Name(), ".json") {
			// Legacy branch-scoped session file
			data, err := os.ReadFile(entryPath)
			if err != nil {
				continue
			}
			var sess Session
			if err := json.Unmarshal(data, &sess); err != nil {
				continue
			}
			sessions = append(sessions, sess)
		}
	}

	return sessions, nil
}

// CleanupStaleSessions removes session files older than maxAge (handles nested structure)
func CleanupStaleSessions(baseDir string, maxAge time.Duration) (int, error) {
	sessionsPath := filepath.Join(baseDir, sessionsDir)

	entries, err := os.ReadDir(sessionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No sessions directory
		}
		return 0, fmt.Errorf("read sessions dir: %w", err)
	}

	now := time.Now()
	deleted := 0

	// Helper to check and delete stale session
	checkAndDelete := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return
		}
		lastActive := sess.LastActivity
		if lastActive.IsZero() {
			lastActive = sess.StartedAt
		}
		if now.Sub(lastActive) > maxAge {
			if err := os.Remove(path); err == nil {
				deleted++
			}
		}
	}

	for _, entry := range entries {
		entryPath := filepath.Join(sessionsPath, entry.Name())

		if entry.IsDir() {
			// Agent-scoped sessions in branch subdirectory
			subEntries, err := os.ReadDir(entryPath)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if subEntry.IsDir() || !strings.HasSuffix(subEntry.Name(), ".json") {
					continue
				}
				checkAndDelete(filepath.Join(entryPath, subEntry.Name()))
			}
			// Remove empty branch directories
			remaining, _ := os.ReadDir(entryPath)
			if len(remaining) == 0 {
				os.Remove(entryPath)
			}
		} else if strings.HasSuffix(entry.Name(), ".json") {
			// Legacy branch-scoped session file
			checkAndDelete(entryPath)
		}
	}

	return deleted, nil
}

// GetCurrentBranch returns the current git branch (exported for display)
func GetCurrentBranch() string {
	return getCurrentBranch()
}
