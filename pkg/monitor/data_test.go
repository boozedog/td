package monitor

import (
	"testing"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
)

func createTestIssue(t *testing.T, database *db.DB, title string, status models.Status) *models.Issue {
	issue := &models.Issue{
		Title:  title,
		Type:   models.TypeTask,
		Status: status,
	}
	if err := database.CreateIssue(issue); err != nil {
		t.Fatalf("failed to create issue %q: %v", title, err)
	}
	return issue
}

func TestComputeBoardIssueCategories(t *testing.T) {
	baseDir := t.TempDir()
	database, err := db.Initialize(baseDir)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Create a blocker issue (open)
	blocker := createTestIssue(t, database, "Blocker issue", models.StatusOpen)

	// Create a blocked issue (open, depends on blocker)
	blocked := createTestIssue(t, database, "Blocked issue", models.StatusOpen)
	if err := database.AddDependency(blocked.ID, blocker.ID, "depends_on"); err != nil {
		t.Fatalf("failed to add dependency: %v", err)
	}

	// Create a ready issue (open, no dependencies)
	ready := createTestIssue(t, database, "Ready issue", models.StatusOpen)

	// Create an explicitly blocked issue
	explicitBlocked := createTestIssue(t, database, "Explicit blocked", models.StatusBlocked)

	// Build BoardIssueViews
	issues := []models.BoardIssueView{
		{Issue: *blocker},
		{Issue: *blocked},
		{Issue: *ready},
		{Issue: *explicitBlocked},
	}

	// Compute categories
	ComputeBoardIssueCategories(database, issues, "test-session")

	// Verify categories
	tests := []struct {
		name     string
		issueID  string
		expected TaskListCategory
	}{
		{"blocker is ready", blocker.ID, CategoryReady},
		{"blocked by dep is blocked", blocked.ID, CategoryBlocked},
		{"ready issue is ready", ready.ID, CategoryReady},
		{"explicit blocked is blocked", explicitBlocked.ID, CategoryBlocked},
	}

	// Build lookup map
	categoryByID := make(map[string]string)
	for _, biv := range issues {
		categoryByID[biv.Issue.ID] = biv.Category
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TaskListCategory(categoryByID[tt.issueID])
			if got != tt.expected {
				t.Errorf("issue %s: got category %q, want %q", tt.issueID, got, tt.expected)
			}
		})
	}
}

func TestComputeBoardIssueCategoriesClosedDepUnblocks(t *testing.T) {
	baseDir := t.TempDir()
	database, err := db.Initialize(baseDir)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Create a blocker issue (closed)
	blocker := createTestIssue(t, database, "Blocker issue", models.StatusClosed)

	// Create dependent issue (should be ready since blocker is closed)
	dependent := createTestIssue(t, database, "Dependent issue", models.StatusOpen)
	if err := database.AddDependency(dependent.ID, blocker.ID, "depends_on"); err != nil {
		t.Fatalf("failed to add dependency: %v", err)
	}

	issues := []models.BoardIssueView{{Issue: *dependent}}

	ComputeBoardIssueCategories(database, issues, "test-session")

	if issues[0].Category != string(CategoryReady) {
		t.Errorf("dependent with closed blocker: got %q, want %q", issues[0].Category, CategoryReady)
	}
}
