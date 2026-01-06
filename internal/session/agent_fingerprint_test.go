package session

import (
	"os"
	"testing"
)

func TestAgentFingerprintString(t *testing.T) {
	tests := []struct {
		name     string
		fp       AgentFingerprint
		expected string
	}{
		{
			name:     "claude-code with PID",
			fp:       AgentFingerprint{Type: AgentClaudeCode, PID: 12345},
			expected: "claude-code_12345",
		},
		{
			name:     "cursor with PID",
			fp:       AgentFingerprint{Type: AgentCursor, PID: 67890},
			expected: "cursor_67890",
		},
		{
			name:     "terminal without PID",
			fp:       AgentFingerprint{Type: AgentTerminal, PID: 0},
			expected: "terminal",
		},
		{
			name:     "unknown without PID",
			fp:       AgentFingerprint{Type: AgentUnknown, PID: 0},
			expected: "unknown",
		},
		{
			name:     "explicit with ID",
			fp:       AgentFingerprint{Type: AgentType("explicit"), PID: 0, ExplicitID: "my-session"},
			expected: "explicit_my-session",
		},
		{
			name:     "explicit with special chars sanitized",
			fp:       AgentFingerprint{Type: AgentType("explicit"), PID: 0, ExplicitID: "session/with:special*chars"},
			expected: "explicit_session_with_special_chars",
		},
		{
			name:     "explicit with long ID truncated",
			fp:       AgentFingerprint{Type: AgentType("explicit"), PID: 0, ExplicitID: "this-is-a-very-long-session-id-that-exceeds-limit"},
			expected: "explicit_this-is-a-very-long-session-id-t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fp.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetAgentFingerprintWithExplicitOverride(t *testing.T) {
	// Set explicit override
	t.Setenv("TD_SESSION_ID", "my-explicit-session")

	fp := GetAgentFingerprint()

	if fp.Type != "explicit" {
		t.Errorf("Type = %q, want %q", fp.Type, "explicit")
	}
	if fp.PID != 0 {
		t.Errorf("PID = %d, want 0 for explicit override", fp.PID)
	}
	if fp.ExplicitID != "my-explicit-session" {
		t.Errorf("ExplicitID = %q, want %q", fp.ExplicitID, "my-explicit-session")
	}
}

func TestGetAgentFingerprintWithCursorEnv(t *testing.T) {
	// Clear explicit override
	os.Unsetenv("TD_SESSION_ID")
	// Set Cursor agent env var
	t.Setenv("CURSOR_AGENT", "1")

	fp := GetAgentFingerprint()

	if fp.Type != AgentCursor {
		t.Errorf("Type = %q, want %q", fp.Type, AgentCursor)
	}
	// PID should be set (parent PID)
	if fp.PID == 0 {
		t.Errorf("PID should be set for Cursor agent")
	}
}

func TestGetAgentFingerprintFallback(t *testing.T) {
	// Clear all agent-related env vars
	os.Unsetenv("TD_SESSION_ID")
	os.Unsetenv("CURSOR_AGENT")

	fp := GetAgentFingerprint()

	// Should detect claude-code or fall back to terminal/unknown
	// In test environment, we're likely running under go test, not an agent
	validTypes := map[AgentType]bool{
		AgentClaudeCode: true,
		AgentCursor:     true,
		AgentCodex:      true,
		AgentWindsurf:   true,
		AgentZed:        true,
		AgentAider:      true,
		AgentCopilot:    true,
		AgentGemini:     true,
		AgentTerminal:   true,
		AgentUnknown:    true,
	}

	if !validTypes[fp.Type] {
		t.Errorf("Type = %q, not a valid agent type", fp.Type)
	}
}

func TestDetectAgentAncestorReturnsUnknownForNoAgent(t *testing.T) {
	// This test verifies the function doesn't panic and returns something
	// In most test environments, we won't find an agent ancestor
	fp := detectAgentAncestor()

	// Should return either a detected agent or unknown
	if fp.Type == "" {
		t.Error("Type should not be empty")
	}
}

func TestGetTerminalSessionID(t *testing.T) {
	// Clear any terminal session vars
	for _, env := range []string{
		"TERM_SESSION_ID",
		"TMUX_PANE",
		"STY",
		"WINDOWID",
		"KONSOLE_DBUS_SESSION",
		"GNOME_TERMINAL_SCREEN",
	} {
		os.Unsetenv(env)
	}

	// Should return empty when no terminal vars set
	result := getTerminalSessionID()
	if result != "" {
		t.Errorf("getTerminalSessionID() = %q, want empty", result)
	}

	// Set a terminal var and verify it's detected
	t.Setenv("TERM_SESSION_ID", "test-terminal-123")
	result = getTerminalSessionID()
	if result != "test-terminal-123" {
		t.Errorf("getTerminalSessionID() = %q, want %q", result, "test-terminal-123")
	}
}

func TestAgentPatternsContainsExpectedAgents(t *testing.T) {
	expectedPatterns := []string{
		"claude",
		"cursor",
		"codex",
		"windsurf",
		"zed",
		"aider",
		"copilot",
		"gemini",
	}

	for _, pattern := range expectedPatterns {
		if _, ok := agentPatterns[pattern]; !ok {
			t.Errorf("agentPatterns missing pattern %q", pattern)
		}
	}
}
