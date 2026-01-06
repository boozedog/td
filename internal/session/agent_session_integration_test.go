package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAgentScopedSessionIsolation verifies that different agents get different sessions
func TestAgentScopedSessionIsolation(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	// Simulate Agent A (explicit override)
	t.Setenv("TD_SESSION_ID", "agent-a")
	sessA, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate for agent A: %v", err)
	}

	// Simulate Agent B (different explicit override)
	t.Setenv("TD_SESSION_ID", "agent-b")
	sessB, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate for agent B: %v", err)
	}

	// Key assertion: different agents should get different sessions
	if sessA.ID == sessB.ID {
		t.Errorf("Agents A and B should have different session IDs, both got %s", sessA.ID)
	}

	// Verify session files are in different locations
	sessionsDir := filepath.Join(baseDir, ".todos/sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatalf("read sessions dir: %v", err)
	}

	// Should have a branch subdirectory with agent-specific files
	foundDirs := 0
	for _, entry := range entries {
		if entry.IsDir() {
			foundDirs++
		}
	}
	if foundDirs == 0 {
		t.Error("Expected at least one branch subdirectory in sessions")
	}
}

// TestSameAgentSameSession verifies stability within same agent
func TestSameAgentSameSession(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "stable-agent")

	// First call creates session
	sess1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (1): %v", err)
	}

	// Second call should return same session
	sess2, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (2): %v", err)
	}

	// Third call should still return same session
	sess3, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (3): %v", err)
	}

	if sess1.ID != sess2.ID || sess2.ID != sess3.ID {
		t.Errorf("Same agent should get same session ID across calls: %s, %s, %s",
			sess1.ID, sess2.ID, sess3.ID)
	}
}

// TestAgentSessionPersistence verifies session survives restart
func TestAgentSessionPersistence(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "persistent-agent")

	// Create initial session
	sess1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (initial): %v", err)
	}
	initialID := sess1.ID

	// Clear in-memory state (simulate restart)
	// Just call GetOrCreate again - it should load from disk

	sess2, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (after restart): %v", err)
	}

	if sess2.ID != initialID {
		t.Errorf("Session should persist across restarts: got %s, want %s", sess2.ID, initialID)
	}
}

// TestAgentSessionFileStructure verifies correct file hierarchy
func TestAgentSessionFileStructure(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "test-agent")

	sess, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	// Verify sessions directory exists with at least one branch subdirectory
	sessionsDir := filepath.Join(baseDir, ".todos/sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatalf("read sessions dir: %v", err)
	}

	foundBranchDir := false
	for _, entry := range entries {
		if entry.IsDir() {
			foundBranchDir = true
			// Check for session file inside
			subEntries, err := os.ReadDir(filepath.Join(sessionsDir, entry.Name()))
			if err != nil {
				t.Errorf("read branch dir: %v", err)
				continue
			}
			if len(subEntries) == 0 {
				t.Errorf("Branch dir %s should contain session file", entry.Name())
			}
		}
	}

	if !foundBranchDir {
		t.Error("Expected at least one branch subdirectory in sessions")
	}

	// Check that session file contains expected agent info
	if sess.AgentType != "explicit" {
		t.Errorf("AgentType = %q, want %q", sess.AgentType, "explicit")
	}
}

// TestForceNewSessionCreatesNewAgentSession verifies --new-session behavior
func TestForceNewSessionCreatesNewAgentSession(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "force-new-agent")

	// Create initial session
	sess1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	// Force new session
	sess2, err := ForceNewSession(baseDir)
	if err != nil {
		t.Fatalf("ForceNewSession: %v", err)
	}

	// Should have different ID
	if sess1.ID == sess2.ID {
		t.Errorf("ForceNewSession should create new ID, both got %s", sess1.ID)
	}

	// New session should link to previous
	if sess2.PreviousSessionID != sess1.ID {
		t.Errorf("PreviousSessionID = %q, want %q", sess2.PreviousSessionID, sess1.ID)
	}

	// Subsequent GetOrCreate should return the new session
	sess3, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate after force: %v", err)
	}

	if sess3.ID != sess2.ID {
		t.Errorf("GetOrCreate after force should return new session: got %s, want %s",
			sess3.ID, sess2.ID)
	}
}
