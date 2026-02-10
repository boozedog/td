package monitor

import (
	"testing"

	"github.com/marcus/td/internal/models"
)

func TestKanbanColumnIssues(t *testing.T) {
	data := TaskListData{
		Reviewable:    []models.Issue{{ID: "r1"}},
		NeedsRework:   []models.Issue{{ID: "nw1"}, {ID: "nw2"}},
		InProgress:    []models.Issue{{ID: "ip1"}},
		Ready:         []models.Issue{{ID: "rd1"}, {ID: "rd2"}, {ID: "rd3"}},
		PendingReview: []models.Issue{{ID: "pr1"}, {ID: "pr2"}},
		Blocked:       nil,
		Closed:        []models.Issue{{ID: "c1"}},
	}

	tests := []struct {
		cat      TaskListCategory
		expected int
	}{
		{CategoryReviewable, 1},
		{CategoryNeedsRework, 2},
		{CategoryInProgress, 1},
		{CategoryReady, 3},
		{CategoryPendingReview, 2},
		{CategoryBlocked, 0},
		{CategoryClosed, 1},
	}

	for _, tt := range tests {
		issues := kanbanColumnIssues(data, tt.cat)
		if len(issues) != tt.expected {
			t.Errorf("kanbanColumnIssues(%s) = %d issues, want %d", tt.cat, len(issues), tt.expected)
		}
	}
}

func TestKanbanNavigation(t *testing.T) {
	// Column order: 0=Review, 1=Rework, 2=InProgress, 3=Ready, 4=PendingReview, 5=Blocked, 6=Closed
	m := Model{
		KanbanOpen: true,
		KanbanCol:  0,
		KanbanRow:  0,
		BoardMode: BoardMode{
			SwimlaneData: TaskListData{
				Reviewable:    []models.Issue{{ID: "r1"}, {ID: "r2"}},
				NeedsRework:   nil,
				InProgress:    []models.Issue{{ID: "ip1"}},
				Ready:         []models.Issue{{ID: "rd1"}},
				PendingReview: nil,
				Blocked:       nil,
				Closed:        []models.Issue{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}},
			},
		},
	}

	// Test move down within column (Reviewable has 2 items)
	m.kanbanMoveDown()
	if m.KanbanRow != 1 {
		t.Errorf("after moveDown: row = %d, want 1", m.KanbanRow)
	}

	// Test move down at bottom of column (should not move)
	m.kanbanMoveDown()
	if m.KanbanRow != 1 {
		t.Errorf("after moveDown at bottom: row = %d, want 1", m.KanbanRow)
	}

	// Test move up
	m.kanbanMoveUp()
	if m.KanbanRow != 0 {
		t.Errorf("after moveUp: row = %d, want 0", m.KanbanRow)
	}

	// Test move up at top (should not move)
	m.kanbanMoveUp()
	if m.KanbanRow != 0 {
		t.Errorf("after moveUp at top: row = %d, want 0", m.KanbanRow)
	}

	// Test move right to col 1 (NeedsRework - empty)
	m.kanbanMoveRight()
	if m.KanbanCol != 1 {
		t.Errorf("after moveRight: col = %d, want 1", m.KanbanCol)
	}
	if m.KanbanRow != 0 {
		t.Errorf("after moveRight to empty col: row = %d, want 0", m.KanbanRow)
	}

	// Move right to InProgress (col 2)
	m.kanbanMoveRight()
	if m.KanbanCol != 2 {
		t.Errorf("after second moveRight: col = %d, want 2", m.KanbanCol)
	}

	// Move all the way to Closed (col 6)
	m.kanbanMoveRight() // col 3 (Ready)
	m.kanbanMoveRight() // col 4 (PendingReview)
	m.kanbanMoveRight() // col 5 (Blocked)
	m.kanbanMoveRight() // col 6 (Closed)
	if m.KanbanCol != 6 {
		t.Errorf("col should be 6, got %d", m.KanbanCol)
	}

	// Move right at rightmost column (should not move)
	m.kanbanMoveRight()
	if m.KanbanCol != 6 {
		t.Errorf("after moveRight at rightmost: col = %d, want 6", m.KanbanCol)
	}

	// Col 6 (Closed) has 3 items - move down to row 2
	m.kanbanMoveDown()
	m.kanbanMoveDown()
	if m.KanbanRow != 2 {
		t.Errorf("after moving down in Closed: row = %d, want 2", m.KanbanRow)
	}

	// Move left to Blocked (col 5, empty) - row should clamp to 0
	m.kanbanMoveLeft()
	if m.KanbanCol != 5 {
		t.Errorf("after moveLeft: col = %d, want 5", m.KanbanCol)
	}
	if m.KanbanRow != 0 {
		t.Errorf("after moveLeft to empty col: row = %d, want 0", m.KanbanRow)
	}

	// Move left to col 0
	m.kanbanMoveLeft() // col 4
	m.kanbanMoveLeft() // col 3
	m.kanbanMoveLeft() // col 2
	m.kanbanMoveLeft() // col 1
	m.kanbanMoveLeft() // col 0
	if m.KanbanCol != 0 {
		t.Errorf("col should be 0, got %d", m.KanbanCol)
	}

	// Move left at leftmost (should not move)
	m.kanbanMoveLeft()
	if m.KanbanCol != 0 {
		t.Errorf("after moveLeft at leftmost: col = %d, want 0", m.KanbanCol)
	}
}

func TestKanbanClampRow(t *testing.T) {
	m := Model{
		KanbanOpen: true,
		KanbanCol:  0,
		KanbanRow:  5, // out of range
		BoardMode: BoardMode{
			SwimlaneData: TaskListData{
				Reviewable: []models.Issue{{ID: "r1"}, {ID: "r2"}},
			},
		},
	}

	m.clampKanbanRow()
	if m.KanbanRow != 1 {
		t.Errorf("clampKanbanRow: row = %d, want 1", m.KanbanRow)
	}

	// Empty column
	m.KanbanCol = 1 // NeedsRework is empty
	m.KanbanRow = 5
	m.clampKanbanRow()
	if m.KanbanRow != 0 {
		t.Errorf("clampKanbanRow on empty col: row = %d, want 0", m.KanbanRow)
	}
}

func TestKanbanColumnLabelsAndColors(t *testing.T) {
	// Verify all columns have labels and colors
	for _, cat := range kanbanColumnOrder {
		label := kanbanColumnLabel(cat)
		if label == "" {
			t.Errorf("kanbanColumnLabel(%s) returned empty string", cat)
		}

		color := kanbanColumnColor(cat)
		if color == "" {
			t.Errorf("kanbanColumnColor(%s) returned empty string", cat)
		}
	}
}
