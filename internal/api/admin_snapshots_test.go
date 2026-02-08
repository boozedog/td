package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestAdminSnapshotMeta_NoSnapshot(t *testing.T) {
	srv, store := newTestServer(t)
	_, adminToken := createTestAdminKey(t, store, "admin@test.com", "admin:read:snapshots,sync")
	_, userToken := createTestUser(t, store, "snap-user@test.com")

	// Create project via API
	w := doRequest(srv, "POST", "/v1/projects", userToken, CreateProjectRequest{Name: "snap-meta-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Push some events
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), userToken, PushRequest{
		DeviceID: "dev1", SessionID: "sess1",
		Events: []EventInput{
			{ClientActionID: 1, ActionType: "create", EntityType: "issues", EntityID: "i_001",
				Payload: json.RawMessage(`{"title":"test"}`), ClientTimestamp: "2025-01-01T00:00:00Z"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Get snapshot meta (no snapshot built yet)
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/admin/projects/%s/snapshot/meta", project.ID), adminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp snapshotMetaResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.HeadSeq < 1 {
		t.Fatalf("expected head_seq >= 1, got %d", resp.HeadSeq)
	}
	if resp.SnapshotSeq != 0 {
		t.Fatalf("expected snapshot_seq 0 (no snapshot), got %d", resp.SnapshotSeq)
	}
	if resp.Staleness != resp.HeadSeq {
		t.Fatalf("expected staleness = head_seq (%d), got %d", resp.HeadSeq, resp.Staleness)
	}
}

func TestAdminSnapshotMeta_WithSnapshot(t *testing.T) {
	srv, store := newTestServer(t)
	_, adminToken := createTestAdminKey(t, store, "admin@test.com", "admin:read:snapshots,sync")
	_, userToken := createTestUser(t, store, "snap-with@test.com")

	// Create project via API
	w := doRequest(srv, "POST", "/v1/projects", userToken, CreateProjectRequest{Name: "snap-with-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Push events
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), userToken, PushRequest{
		DeviceID: "dev1", SessionID: "sess1",
		Events: []EventInput{
			{ClientActionID: 1, ActionType: "create", EntityType: "issues", EntityID: "i_001",
				Payload: json.RawMessage(`{"schema_version":1,"new_data":{"title":"one","status":"open"}}`), ClientTimestamp: "2025-01-01T00:00:00Z"},
			{ClientActionID: 2, ActionType: "create", EntityType: "issues", EntityID: "i_002",
				Payload: json.RawMessage(`{"schema_version":1,"new_data":{"title":"two","status":"open"}}`), ClientTimestamp: "2025-01-01T00:00:01Z"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Build a snapshot (by requesting the sync snapshot endpoint which caches it)
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/snapshot", project.ID), userToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Now get admin snapshot meta
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/admin/projects/%s/snapshot/meta", project.ID), adminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp snapshotMetaResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.SnapshotSeq < 2 {
		t.Fatalf("expected snapshot_seq >= 2, got %d", resp.SnapshotSeq)
	}
	if resp.HeadSeq < 2 {
		t.Fatalf("expected head_seq >= 2, got %d", resp.HeadSeq)
	}
	if resp.Staleness != 0 {
		t.Fatalf("expected staleness 0 (snapshot is current), got %d", resp.Staleness)
	}
	// Should have entity counts
	if resp.EntityCounts["issues"] != 2 {
		t.Fatalf("expected 2 issues in snapshot, got %d", resp.EntityCounts["issues"])
	}
}

func TestAdminSnapshotMeta_NotFound(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestAdminKey(t, store, "admin@test.com", "admin:read:snapshots,sync")

	w := doRequest(srv, "GET", "/v1/admin/projects/nonexistent/snapshot/meta", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminSnapshotMeta_RequiresAdmin(t *testing.T) {
	srv, store := newTestServer(t)
	_, _ = store.CreateUser("first@test.com")
	_, token := createTestUser(t, store, "nonadmin@test.com")

	w := doRequest(srv, "GET", "/v1/admin/projects/fake/snapshot/meta", token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminSnapshotMeta_RequiresScope(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestAdminKey(t, store, "admin@test.com", "admin:read:server")

	w := doRequest(srv, "GET", "/v1/admin/projects/fake/snapshot/meta", token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (wrong scope), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminSnapshotQuery_NotImplemented(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestAdminKey(t, store, "admin@test.com", "admin:read:snapshots,sync")

	w := doRequest(srv, "GET", "/v1/admin/projects/fake/snapshot/query?q=status+%3D+open", token, nil)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Code != "not_implemented" {
		t.Fatalf("expected error code not_implemented, got %q", resp.Error.Code)
	}
}
