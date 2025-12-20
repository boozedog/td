package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetOrCreateRequiresInit(t *testing.T) {
	baseDir := t.TempDir()

	_, err := GetOrCreate(baseDir)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "td init") {
		t.Fatalf("expected error to mention td init, got: %v", err)
	}
}

func TestGetOrCreateReusesSessionWhenContextStable(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "ctx-1")

	s1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if s1.ID == "" {
		t.Fatalf("expected session ID")
	}
	if !s1.IsNew {
		t.Fatalf("expected IsNew=true on first create")
	}

	s2, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (second): %v", err)
	}
	if s2.IsNew {
		t.Fatalf("expected IsNew=false when reusing existing session")
	}
	if s1.ID != s2.ID {
		t.Fatalf("expected same session ID, got %q vs %q", s1.ID, s2.ID)
	}
}

func TestGetOrCreateRotatesOnContextChange(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "ctx-1")
	s1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "ctx-2")
	s2, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate (rotated): %v", err)
	}
	if s1.ID == s2.ID {
		t.Fatalf("expected new session ID on context change")
	}
	if s2.PreviousSessionID != s1.ID {
		t.Fatalf("expected PreviousSessionID=%q, got %q", s1.ID, s2.PreviousSessionID)
	}
	if !s2.IsNew {
		t.Fatalf("expected IsNew=true for newly created session")
	}
}

func TestForceNewSessionRequiresInit(t *testing.T) {
	baseDir := t.TempDir()

	_, err := ForceNewSession(baseDir)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "td init") {
		t.Fatalf("expected error to mention td init, got: %v", err)
	}
}

func TestForceNewSessionAlwaysCreatesNew(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".todos"), 0755); err != nil {
		t.Fatalf("mkdir .todos: %v", err)
	}

	t.Setenv("TD_SESSION_ID", "ctx-1")
	s1, err := GetOrCreate(baseDir)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	s2, err := ForceNewSession(baseDir)
	if err != nil {
		t.Fatalf("ForceNewSession: %v", err)
	}
	if s1.ID == s2.ID {
		t.Fatalf("expected different session IDs")
	}
	if s2.PreviousSessionID != s1.ID {
		t.Fatalf("expected PreviousSessionID=%q, got %q", s1.ID, s2.PreviousSessionID)
	}
	if !s2.IsNew {
		t.Fatalf("expected IsNew=true for newly created session")
	}
}
