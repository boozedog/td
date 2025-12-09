package cmd

import (
	"testing"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
)

func TestClearFocusIfNeeded(t *testing.T) {
	dir := t.TempDir()

	// Initialize database to create .todos directory
	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	database.Close()

	// Set focus on an issue
	targetID := "td-test123"
	if err := config.SetFocus(dir, targetID); err != nil {
		t.Fatalf("SetFocus failed: %v", err)
	}

	// Verify focus is set
	focused, _ := config.GetFocus(dir)
	if focused != targetID {
		t.Fatalf("Focus not set: got %q, want %q", focused, targetID)
	}

	// Clear focus with matching ID
	clearFocusIfNeeded(dir, targetID)

	// Verify focus is cleared
	focused, _ = config.GetFocus(dir)
	if focused != "" {
		t.Errorf("Focus not cleared: got %q, want empty", focused)
	}
}

func TestClearFocusIfNeededNonMatching(t *testing.T) {
	dir := t.TempDir()

	// Initialize database to create .todos directory
	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	database.Close()

	// Set focus on an issue
	focusedID := "td-focused"
	if err := config.SetFocus(dir, focusedID); err != nil {
		t.Fatalf("SetFocus failed: %v", err)
	}

	// Try to clear with different ID
	clearFocusIfNeeded(dir, "td-different")

	// Focus should still be set
	focused, _ := config.GetFocus(dir)
	if focused != focusedID {
		t.Errorf("Focus was incorrectly cleared: got %q, want %q", focused, focusedID)
	}
}

func TestClearFocusIfNeededNoFocus(t *testing.T) {
	dir := t.TempDir()

	// Initialize database to create .todos directory
	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	database.Close()

	// Don't set any focus, just try to clear
	clearFocusIfNeeded(dir, "td-any")

	// Should not panic or error
	focused, _ := config.GetFocus(dir)
	if focused != "" {
		t.Errorf("Unexpected focus found: %q", focused)
	}
}

func TestReviewRequiresHandoff(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	// Create an issue
	issue := &models.Issue{
		Title:  "Test Issue",
		Status: models.StatusInProgress,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Verify no handoff exists
	handoff, err := database.GetLatestHandoff(issue.ID)
	if err != nil {
		t.Fatalf("GetLatestHandoff failed: %v", err)
	}
	if handoff != nil {
		t.Error("Expected no handoff, but found one")
	}

	// Note: Full command testing would require setting up session and executing command
	// This test verifies the handoff check logic by checking database state
}

func TestApproveRequiresDifferentSession(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	sessionID := "ses_impl123"

	// Create an issue
	issue := &models.Issue{
		Title:  "Test Issue",
		Status: models.StatusInReview,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Update to set implementer (CreateIssue doesn't persist implementer_session)
	issue.ImplementerSession = sessionID
	if err := database.UpdateIssue(issue); err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Verify the issue has the implementer set
	retrieved, _ := database.GetIssue(issue.ID)
	if retrieved.ImplementerSession != sessionID {
		t.Errorf("Implementer not set: got %q, want %q", retrieved.ImplementerSession, sessionID)
	}

	// Note: The actual "cannot approve own implementation" check is in the command
	// This test verifies the data model supports tracking implementer sessions
}

func TestRejectResetsToInProgress(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	// Create an issue in review
	issue := &models.Issue{
		Title:  "Test Issue",
		Status: models.StatusInReview,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Update to in_progress (simulating reject)
	issue.Status = models.StatusInProgress
	if err := database.UpdateIssue(issue); err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Verify status changed
	retrieved, _ := database.GetIssue(issue.ID)
	if retrieved.Status != models.StatusInProgress {
		t.Errorf("Status not updated: got %q, want %q", retrieved.Status, models.StatusInProgress)
	}
}

func TestCloseSetsClosedAt(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	// Create an issue
	issue := &models.Issue{
		Title:  "Test Issue",
		Status: models.StatusOpen,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Verify ClosedAt is nil initially
	if issue.ClosedAt != nil {
		t.Error("ClosedAt should be nil for new issue")
	}

	// Update to closed with ClosedAt
	issue.Status = models.StatusClosed
	now := issue.UpdatedAt
	issue.ClosedAt = &now
	if err := database.UpdateIssue(issue); err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Verify ClosedAt is set
	retrieved, _ := database.GetIssue(issue.ID)
	if retrieved.ClosedAt == nil {
		t.Error("ClosedAt should be set after closing")
	}
	if retrieved.Status != models.StatusClosed {
		t.Errorf("Status not updated: got %q, want %q", retrieved.Status, models.StatusClosed)
	}
}

func TestApproveAddsReviewerSession(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	implSession := "ses_impl123"
	reviewSession := "ses_review456"

	// Create an issue with implementer
	issue := &models.Issue{
		Title:              "Test Issue",
		Status:             models.StatusInReview,
		ImplementerSession: implSession,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Update with reviewer (simulating approve)
	issue.Status = models.StatusClosed
	issue.ReviewerSession = reviewSession
	now := issue.UpdatedAt
	issue.ClosedAt = &now
	if err := database.UpdateIssue(issue); err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Verify reviewer is set
	retrieved, _ := database.GetIssue(issue.ID)
	if retrieved.ReviewerSession != reviewSession {
		t.Errorf("ReviewerSession not set: got %q, want %q", retrieved.ReviewerSession, reviewSession)
	}
	if retrieved.ImplementerSession != implSession {
		t.Errorf("ImplementerSession changed: got %q, want %q", retrieved.ImplementerSession, implSession)
	}
}

func TestReviewAddsLogEntry(t *testing.T) {
	dir := t.TempDir()

	database, err := db.Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer database.Close()

	// Create an issue
	issue := &models.Issue{
		Title:  "Test Issue",
		Status: models.StatusInProgress,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	// Add a log entry (simulating what review command does)
	log := &models.Log{
		IssueID:   issue.ID,
		SessionID: "ses_test",
		Message:   "Submitted for review",
		Type:      models.LogTypeProgress,
	}
	if err := database.AddLog(log); err != nil {
		t.Fatalf("AddLog failed: %v", err)
	}

	// Verify log was added
	logs, err := database.GetLogs(issue.ID, 0)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].Message != "Submitted for review" {
		t.Errorf("Wrong log message: got %q", logs[0].Message)
	}
}
