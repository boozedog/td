package db

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marcus/td/internal/models"
)

func TestImportExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcDB, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize source DB: %v", err)
	}

	// Create issue with non-default fields
	closedAt := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	deferUntil := "2025-04-01"
	dueDate := "2025-05-01"

	issue := &models.Issue{
		Title:              "Test import round-trip",
		Description:        "Detailed description",
		Status:             models.StatusInReview,
		Type:               models.TypeBug,
		Priority:           models.PriorityP1,
		Points:             5,
		Labels:             []string{"backend", "urgent"},
		ParentID:           "td-parent1",
		Acceptance:         "All tests pass",
		Sprint:             "sprint-5",
		ImplementerSession: "ses_impl",
		CreatorSession:     "ses_creator",
		ReviewerSession:    "ses_reviewer",
		Minor:              true,
		CreatedBranch:      "fix/import",
		DeferUntil:         &deferUntil,
		DueDate:            &dueDate,
	}
	if err := srcDB.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	// Manually set ClosedAt and status via UpsertIssueRaw since CreateIssue won't set closed_at
	issue.ClosedAt = &closedAt
	if err := srcDB.UpsertIssueRaw(issue); err != nil {
		t.Fatalf("UpsertIssueRaw: %v", err)
	}

	// Add log entry
	log := &models.Log{
		IssueID:   issue.ID,
		SessionID: "ses_impl",
		Message:   "Fixed the bug",
		Type:      models.LogTypeProgress,
	}
	if err := srcDB.AddLog(log); err != nil {
		t.Fatalf("AddLog: %v", err)
	}

	// Add handoff
	handoff := &models.Handoff{
		IssueID:   issue.ID,
		SessionID: "ses_impl",
		Done:      []string{"fixed parsing"},
		Remaining: []string{"add tests"},
		Decisions: []string{"use json.Unmarshal"},
	}
	if err := srcDB.AddHandoff(handoff); err != nil {
		t.Fatalf("AddHandoff: %v", err)
	}

	// Export from source
	issues, err := srcDB.ListIssues(ListIssuesOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	exportData := make([]map[string]interface{}, 0)
	for _, iss := range issues {
		logs, _ := srcDB.GetLogs(iss.ID, 0)
		ho, _ := srcDB.GetLatestHandoff(iss.ID)
		deps, _ := srcDB.GetDependencies(iss.ID)
		files, _ := srcDB.GetLinkedFiles(iss.ID)

		item := map[string]interface{}{
			"issue":        iss,
			"logs":         logs,
			"handoff":      ho,
			"dependencies": deps,
			"files":        files,
		}
		exportData = append(exportData, item)
	}
	data, err := json.Marshal(exportData)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	srcDB.Close()

	// Import into fresh DB
	dir2 := t.TempDir()
	dstDB, err := Initialize(dir2)
	if err != nil {
		t.Fatalf("Initialize dest DB: %v", err)
	}
	defer dstDB.Close()

	// Parse and import
	var importData []map[string]json.RawMessage
	if err := json.Unmarshal(data, &importData); err != nil {
		t.Fatalf("Unmarshal import data: %v", err)
	}
	for _, item := range importData {
		var imp models.Issue
		if err := json.Unmarshal(item["issue"], &imp); err != nil {
			t.Fatalf("Unmarshal issue: %v", err)
		}
		if err := dstDB.UpsertIssueRaw(&imp); err != nil {
			t.Fatalf("UpsertIssueRaw: %v", err)
		}

		// Import logs
		if logsRaw, ok := item["logs"]; ok {
			var logs []models.Log
			if err := json.Unmarshal(logsRaw, &logs); err == nil {
				for i := range logs {
					if err := dstDB.AddLog(&logs[i]); err != nil {
						t.Fatalf("AddLog: %v", err)
					}
				}
			}
		}

		// Import handoff
		if handoffRaw, ok := item["handoff"]; ok && string(handoffRaw) != "null" {
			var ho models.Handoff
			if err := json.Unmarshal(handoffRaw, &ho); err == nil && ho.IssueID != "" {
				if err := dstDB.AddHandoff(&ho); err != nil {
					t.Fatalf("AddHandoff: %v", err)
				}
			}
		}
	}

	// Verify the imported issue
	got, err := dstDB.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue after import: %v", err)
	}

	// Check all fields
	if got.Title != issue.Title {
		t.Errorf("Title = %q, want %q", got.Title, issue.Title)
	}
	if got.Description != issue.Description {
		t.Errorf("Description = %q, want %q", got.Description, issue.Description)
	}
	if got.Status != models.StatusInReview {
		t.Errorf("Status = %q, want %q", got.Status, models.StatusInReview)
	}
	if got.Type != models.TypeBug {
		t.Errorf("Type = %q, want %q", got.Type, models.TypeBug)
	}
	if got.Priority != models.PriorityP1 {
		t.Errorf("Priority = %q, want %q", got.Priority, models.PriorityP1)
	}
	if got.Points != 5 {
		t.Errorf("Points = %d, want 5", got.Points)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "backend" || got.Labels[1] != "urgent" {
		t.Errorf("Labels = %v, want [backend urgent]", got.Labels)
	}
	if got.ParentID != "td-parent1" {
		t.Errorf("ParentID = %q, want %q", got.ParentID, "td-parent1")
	}
	if got.Acceptance != "All tests pass" {
		t.Errorf("Acceptance = %q, want %q", got.Acceptance, "All tests pass")
	}
	if got.Sprint != "sprint-5" {
		t.Errorf("Sprint = %q, want %q", got.Sprint, "sprint-5")
	}
	if got.ImplementerSession != "ses_impl" {
		t.Errorf("ImplementerSession = %q, want %q", got.ImplementerSession, "ses_impl")
	}
	if got.CreatorSession != "ses_creator" {
		t.Errorf("CreatorSession = %q, want %q", got.CreatorSession, "ses_creator")
	}
	if got.ReviewerSession != "ses_reviewer" {
		t.Errorf("ReviewerSession = %q, want %q", got.ReviewerSession, "ses_reviewer")
	}
	if got.ClosedAt == nil {
		t.Error("ClosedAt is nil, want non-nil")
	}
	if !got.Minor {
		t.Error("Minor = false, want true")
	}
	if got.CreatedBranch != "fix/import" {
		t.Errorf("CreatedBranch = %q, want %q", got.CreatedBranch, "fix/import")
	}
	if got.DeferUntil == nil || *got.DeferUntil != "2025-04-01" {
		t.Errorf("DeferUntil = %v, want 2025-04-01", got.DeferUntil)
	}
	if got.DueDate == nil || *got.DueDate != "2025-05-01" {
		t.Errorf("DueDate = %v, want 2025-05-01", got.DueDate)
	}

	// Verify logs were imported
	logs, err := dstDB.GetLogs(issue.ID, 0)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) == 0 {
		t.Error("expected at least 1 log entry after import")
	} else {
		found := false
		for _, l := range logs {
			if l.Message == "Fixed the bug" {
				found = true
				break
			}
		}
		if !found {
			t.Error("log message 'Fixed the bug' not found after import")
		}
	}

	// Verify handoff was imported
	ho, err := dstDB.GetLatestHandoff(issue.ID)
	if err != nil {
		t.Fatalf("GetLatestHandoff: %v", err)
	}
	if ho == nil {
		t.Error("expected handoff after import, got nil")
	} else {
		if len(ho.Done) == 0 || ho.Done[0] != "fixed parsing" {
			t.Errorf("handoff.Done = %v, want [fixed parsing]", ho.Done)
		}
		if len(ho.Remaining) == 0 || ho.Remaining[0] != "add tests" {
			t.Errorf("handoff.Remaining = %v, want [add tests]", ho.Remaining)
		}
	}
}

func TestUpsertIssueRaw_PreservesAllFields(t *testing.T) {
	dir := t.TempDir()
	database, err := Initialize(dir)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer database.Close()

	closedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC)
	deferUntil := "2025-07-01"
	dueDate := "2025-08-01"

	issue := &models.Issue{
		ID:                 "td-import1",
		Title:              "Raw upsert test",
		Description:        "Should preserve everything",
		Status:             models.StatusBlocked,
		Type:               models.TypeFeature,
		Priority:           models.PriorityP0,
		Points:             13,
		Labels:             []string{"a", "b", "c"},
		ParentID:           "td-parent2",
		Acceptance:         "criteria here",
		Sprint:             "sprint-10",
		ImplementerSession: "ses_a",
		CreatorSession:     "ses_b",
		ReviewerSession:    "ses_c",
		CreatedAt:          time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		ClosedAt:           &closedAt,
		DeletedAt:          &deletedAt,
		Minor:              true,
		CreatedBranch:      "feat/test",
		DeferUntil:         &deferUntil,
		DueDate:            &dueDate,
		DeferCount:         3,
	}

	if err := database.UpsertIssueRaw(issue); err != nil {
		t.Fatalf("UpsertIssueRaw: %v", err)
	}

	got, err := database.GetIssue("td-import1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	if got.Status != models.StatusBlocked {
		t.Errorf("Status = %q, want %q", got.Status, models.StatusBlocked)
	}
	if got.DeferCount != 3 {
		t.Errorf("DeferCount = %d, want 3", got.DeferCount)
	}
	if got.ClosedAt == nil {
		t.Error("ClosedAt is nil")
	}
	if got.DeletedAt == nil {
		t.Error("DeletedAt is nil")
	}
	if got.CreatedAt.Year() != 2025 || got.CreatedAt.Month() != 1 {
		t.Errorf("CreatedAt = %v, want 2025-01", got.CreatedAt)
	}

	// Upsert again (replace) with changed status
	issue.Status = models.StatusClosed
	if err := database.UpsertIssueRaw(issue); err != nil {
		t.Fatalf("UpsertIssueRaw replace: %v", err)
	}

	got2, err := database.GetIssue("td-import1")
	if err != nil {
		t.Fatalf("GetIssue after replace: %v", err)
	}
	if got2.Status != models.StatusClosed {
		t.Errorf("Status after replace = %q, want %q", got2.Status, models.StatusClosed)
	}
}
