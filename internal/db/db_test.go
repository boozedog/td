package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/marcus/td/internal/models"
)

func TestInitialize(t *testing.T) {
	dir := t.TempDir()

	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	// Check database file exists
	dbPath := filepath.Join(dir, ".todos", "issues.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file not created")
	}
}

func TestCreateAndGetIssue(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	issue := &models.Issue{
		Title:       "Test Issue",
		Description: "Test description",
		Type:        models.TypeBug,
		Priority:    models.PriorityP1,
		Points:      5,
		Labels:      []string{"test", "bug"},
	}

	err = db.CreateIssue(issue)
	if err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	if issue.ID == "" {
		t.Error("Issue ID not set")
	}

	// Retrieve issue
	retrieved, err := db.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}

	if retrieved.Title != issue.Title {
		t.Errorf("Title mismatch: got %s, want %s", retrieved.Title, issue.Title)
	}

	if retrieved.Type != issue.Type {
		t.Errorf("Type mismatch: got %s, want %s", retrieved.Type, issue.Type)
	}

	if retrieved.Priority != issue.Priority {
		t.Errorf("Priority mismatch: got %s, want %s", retrieved.Priority, issue.Priority)
	}

	if len(retrieved.Labels) != 2 {
		t.Errorf("Labels count mismatch: got %d, want 2", len(retrieved.Labels))
	}
}

func TestListIssues(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	// Create test issues
	issues := []struct {
		title    string
		status   models.Status
		priority models.Priority
	}{
		{"Issue 1", models.StatusOpen, models.PriorityP1},
		{"Issue 2", models.StatusOpen, models.PriorityP2},
		{"Issue 3", models.StatusInProgress, models.PriorityP1},
		{"Issue 4", models.StatusClosed, models.PriorityP3},
	}

	for _, tc := range issues {
		issue := &models.Issue{
			Title:    tc.title,
			Status:   tc.status,
			Priority: tc.priority,
		}
		if err := db.CreateIssue(issue); err != nil {
			t.Fatalf("CreateIssue failed: %v", err)
		}
	}

	// Test listing all
	all, err := db.ListIssues(ListIssuesOptions{})
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("Expected 4 issues, got %d", len(all))
	}

	// Test status filter
	open, err := db.ListIssues(ListIssuesOptions{
		Status: []models.Status{models.StatusOpen},
	})
	if err != nil {
		t.Fatalf("ListIssues with status filter failed: %v", err)
	}
	if len(open) != 2 {
		t.Errorf("Expected 2 open issues, got %d", len(open))
	}

	// Test priority filter
	p1, err := db.ListIssues(ListIssuesOptions{
		Priority: "P1",
	})
	if err != nil {
		t.Fatalf("ListIssues with priority filter failed: %v", err)
	}
	if len(p1) != 2 {
		t.Errorf("Expected 2 P1 issues, got %d", len(p1))
	}
}

func TestDeleteAndRestore(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	issue := &models.Issue{Title: "To Delete"}
	if err := db.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Delete
	if err := db.DeleteIssue(issue.ID); err != nil {
		t.Fatalf("DeleteIssue failed: %v", err)
	}

	// Should not appear in normal list
	all, _ := db.ListIssues(ListIssuesOptions{})
	if len(all) != 0 {
		t.Error("Deleted issue still appears in list")
	}

	// Should appear in deleted list
	deleted, _ := db.ListIssues(ListIssuesOptions{OnlyDeleted: true})
	if len(deleted) != 1 {
		t.Error("Deleted issue not in deleted list")
	}

	// Restore
	if err := db.RestoreIssue(issue.ID); err != nil {
		t.Fatalf("RestoreIssue failed: %v", err)
	}

	// Should appear in normal list again
	all, _ = db.ListIssues(ListIssuesOptions{})
	if len(all) != 1 {
		t.Error("Restored issue not in list")
	}
}

func TestLogs(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	issue := &models.Issue{Title: "Test Issue"}
	if err := db.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Add logs
	log1 := &models.Log{
		IssueID:   issue.ID,
		SessionID: "ses_test",
		Message:   "First log",
		Type:      models.LogTypeProgress,
	}
	if err := db.AddLog(log1); err != nil {
		t.Fatalf("AddLog failed: %v", err)
	}

	log2 := &models.Log{
		IssueID:   issue.ID,
		SessionID: "ses_test",
		Message:   "Second log",
		Type:      models.LogTypeHypothesis,
	}
	if err := db.AddLog(log2); err != nil {
		t.Fatalf("AddLog failed: %v", err)
	}

	// Get logs
	logs, err := db.GetLogs(issue.ID, 0)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}

	// Test limit
	limited, _ := db.GetLogs(issue.ID, 1)
	if len(limited) != 1 {
		t.Errorf("Expected 1 log with limit, got %d", len(limited))
	}
}

func TestHandoff(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	issue := &models.Issue{Title: "Test Issue"}
	if err := db.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Add handoff
	handoff := &models.Handoff{
		IssueID:   issue.ID,
		SessionID: "ses_test",
		Done:      []string{"Task 1", "Task 2"},
		Remaining: []string{"Task 3"},
		Decisions: []string{"Decision 1"},
		Uncertain: []string{"Question 1"},
	}
	if err := db.AddHandoff(handoff); err != nil {
		t.Fatalf("AddHandoff failed: %v", err)
	}

	// Get handoff
	retrieved, err := db.GetLatestHandoff(issue.ID)
	if err != nil {
		t.Fatalf("GetLatestHandoff failed: %v", err)
	}

	if len(retrieved.Done) != 2 {
		t.Errorf("Expected 2 done items, got %d", len(retrieved.Done))
	}

	if len(retrieved.Remaining) != 1 {
		t.Errorf("Expected 1 remaining item, got %d", len(retrieved.Remaining))
	}
}

func TestDependencies(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	// Create issues
	issue1 := &models.Issue{Title: "Issue 1"}
	issue2 := &models.Issue{Title: "Issue 2"}
	if err := db.CreateIssue(issue1); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}
	if err := db.CreateIssue(issue2); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Add dependency: issue2 depends on issue1
	if err := db.AddDependency(issue2.ID, issue1.ID, "depends_on"); err != nil {
		t.Fatalf("AddDependency failed: %v", err)
	}

	// Check dependencies
	deps, err := db.GetDependencies(issue2.ID)
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 1 || deps[0] != issue1.ID {
		t.Error("Dependency not correctly stored")
	}

	// Check blocked by
	blocked, err := db.GetBlockedBy(issue1.ID)
	if err != nil {
		t.Fatalf("GetBlockedBy failed: %v", err)
	}
	if len(blocked) != 1 || blocked[0] != issue2.ID {
		t.Error("Blocked by not correctly retrieved")
	}

	// Remove dependency
	if err := db.RemoveDependency(issue2.ID, issue1.ID); err != nil {
		t.Fatalf("RemoveDependency failed: %v", err)
	}

	deps, _ = db.GetDependencies(issue2.ID)
	if len(deps) != 0 {
		t.Error("Dependency not removed")
	}
}

func TestWorkSession(t *testing.T) {
	dir := t.TempDir()
	db, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer db.Close()

	// Create work session
	ws := &models.WorkSession{
		Name:      "Test Session",
		SessionID: "ses_test",
	}
	if err := db.CreateWorkSession(ws); err != nil {
		t.Fatalf("CreateWorkSession failed: %v", err)
	}

	if ws.ID == "" {
		t.Error("Work session ID not set")
	}

	// Create issue and tag it
	issue := &models.Issue{Title: "Test Issue"}
	if err := db.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	if err := db.TagIssueToWorkSession(ws.ID, issue.ID); err != nil {
		t.Fatalf("TagIssueToWorkSession failed: %v", err)
	}

	// Get tagged issues
	issues, err := db.GetWorkSessionIssues(ws.ID)
	if err != nil {
		t.Fatalf("GetWorkSessionIssues failed: %v", err)
	}
	if len(issues) != 1 || issues[0] != issue.ID {
		t.Error("Issue not correctly tagged to work session")
	}

	// Untag
	if err := db.UntagIssueFromWorkSession(ws.ID, issue.ID); err != nil {
		t.Fatalf("UntagIssueFromWorkSession failed: %v", err)
	}

	issues, _ = db.GetWorkSessionIssues(ws.ID)
	if len(issues) != 0 {
		t.Error("Issue not untagged from work session")
	}
}
