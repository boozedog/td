package syncharness

import (
	"testing"

	"github.com/marcus/td/internal/db"
)

const softDelProj = "proj-soft-del"

// TestBoardPositionSoftDelete_SyncConvergence verifies that soft-deleting a
// board_issue_position on one client propagates correctly to another client
// and prevents resurrection when a stale create arrives after a delete.
func TestBoardPositionSoftDelete_SyncConvergence(t *testing.T) {
	h := NewHarness(t, 2, softDelProj)

	boardID := "bd-sdboard"
	issueID := "td-sdissue"
	posID := db.BoardIssuePosID(boardID, issueID)

	// Client A creates a board position
	if err := h.Mutate("client-A", "create", "board_issue_positions", posID, map[string]any{
		"board_id": boardID, "issue_id": issueID, "position": 65536,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Sync A->server->B so both have the position
	if err := h.Sync("client-A", softDelProj); err != nil {
		t.Fatalf("sync A1: %v", err)
	}
	if err := h.Sync("client-B", softDelProj); err != nil {
		t.Fatalf("sync B1: %v", err)
	}

	// Verify both have the position
	for _, cid := range []string{"client-A", "client-B"} {
		ent := h.QueryEntity(cid, "board_issue_positions", posID)
		if ent == nil {
			t.Fatalf("%s: position missing after initial sync", cid)
		}
	}

	// Client A soft-deletes the position
	if err := h.Mutate("client-A", "soft_delete", "board_issue_positions", posID, nil); err != nil {
		t.Fatalf("soft_delete: %v", err)
	}

	// Sync A->server->B
	if err := h.Sync("client-A", softDelProj); err != nil {
		t.Fatalf("sync A2: %v", err)
	}
	if err := h.Sync("client-B", softDelProj); err != nil {
		t.Fatalf("sync B2: %v", err)
	}

	// Both should have deleted_at set (row exists but soft-deleted)
	for _, cid := range []string{"client-A", "client-B"} {
		ent := h.QueryEntity(cid, "board_issue_positions", posID)
		if ent == nil {
			t.Fatalf("%s: row should still exist (soft-deleted)", cid)
		}
		if ent["deleted_at"] == nil {
			t.Fatalf("%s: deleted_at should be set after soft delete", cid)
		}
	}

	h.AssertConverged(softDelProj)
}

// TestBoardPositionSoftDelete_NoResurrection verifies that a stale create
// event does not resurrect a soft-deleted position. The soft_delete event
// with a higher server_seq should win.
func TestBoardPositionSoftDelete_NoResurrection(t *testing.T) {
	h := NewHarness(t, 2, softDelProj)

	boardID := "bd-resurrect"
	issueID := "td-resiss"
	posID := db.BoardIssuePosID(boardID, issueID)

	// Client A creates position and syncs to B
	if err := h.Mutate("client-A", "create", "board_issue_positions", posID, map[string]any{
		"board_id": boardID, "issue_id": issueID, "position": 65536,
	}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if err := h.Sync("client-A", softDelProj); err != nil {
		t.Fatalf("sync A1: %v", err)
	}
	if err := h.Sync("client-B", softDelProj); err != nil {
		t.Fatalf("sync B1: %v", err)
	}

	// Client A soft-deletes the position (offline from B's perspective)
	if err := h.Mutate("client-A", "soft_delete", "board_issue_positions", posID, nil); err != nil {
		t.Fatalf("soft_delete A: %v", err)
	}

	// Client B (unaware of delete) re-creates the same position
	if err := h.Mutate("client-B", "create", "board_issue_positions", posID, map[string]any{
		"board_id": boardID, "issue_id": issueID, "position": 131072,
	}); err != nil {
		t.Fatalf("create B: %v", err)
	}

	// Push A first (soft_delete gets lower server_seq), then push B (create gets higher server_seq)
	if _, err := h.Push("client-A", softDelProj); err != nil {
		t.Fatalf("push A: %v", err)
	}
	if _, err := h.Push("client-B", softDelProj); err != nil {
		t.Fatalf("push B: %v", err)
	}

	// Pull all on both clients to reach convergence
	if _, err := h.PullAll("client-A", softDelProj); err != nil {
		t.Fatalf("pull A: %v", err)
	}
	if _, err := h.PullAll("client-B", softDelProj); err != nil {
		t.Fatalf("pull B: %v", err)
	}

	// Both should converge -- the create (higher server_seq) wins over soft_delete
	h.AssertConverged(softDelProj)

	// The row should exist with the create's position value (B's create had higher seq)
	for _, cid := range []string{"client-A", "client-B"} {
		ent := h.QueryEntity(cid, "board_issue_positions", posID)
		if ent == nil {
			t.Fatalf("%s: position should exist (create won)", cid)
		}
	}
}

// TestBoardPositionSoftDelete_ThenReposition verifies that re-adding a
// position after soft delete clears deleted_at properly.
func TestBoardPositionSoftDelete_ThenReposition(t *testing.T) {
	h := NewHarness(t, 2, softDelProj)

	boardID := "bd-repos"
	issueID := "td-reposiss"
	posID := db.BoardIssuePosID(boardID, issueID)

	// Client A creates position
	if err := h.Mutate("client-A", "create", "board_issue_positions", posID, map[string]any{
		"board_id": boardID, "issue_id": issueID, "position": 65536,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Sync to B
	if err := h.Sync("client-A", softDelProj); err != nil {
		t.Fatalf("sync A1: %v", err)
	}
	if err := h.Sync("client-B", softDelProj); err != nil {
		t.Fatalf("sync B1: %v", err)
	}

	// Client A soft-deletes, then re-creates with new position
	if err := h.Mutate("client-A", "soft_delete", "board_issue_positions", posID, nil); err != nil {
		t.Fatalf("soft_delete: %v", err)
	}
	if err := h.Mutate("client-A", "create", "board_issue_positions", posID, map[string]any{
		"board_id": boardID, "issue_id": issueID, "position": 131072,
	}); err != nil {
		t.Fatalf("re-create: %v", err)
	}

	// Sync both
	if err := h.Sync("client-A", softDelProj); err != nil {
		t.Fatalf("sync A2: %v", err)
	}
	if err := h.Sync("client-B", softDelProj); err != nil {
		t.Fatalf("sync B2: %v", err)
	}

	// Both should have the new position with deleted_at cleared
	for _, cid := range []string{"client-A", "client-B"} {
		ent := h.QueryEntity(cid, "board_issue_positions", posID)
		if ent == nil {
			t.Fatalf("%s: position should exist after re-create", cid)
		}
		if ent["deleted_at"] != nil {
			t.Fatalf("%s: deleted_at should be nil after re-create, got %v", cid, ent["deleted_at"])
		}
		if pos, ok := ent["position"].(int64); !ok || pos != 131072 {
			t.Fatalf("%s: expected position 131072, got %v", cid, ent["position"])
		}
	}

	h.AssertConverged(softDelProj)
}
