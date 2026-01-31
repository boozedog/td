package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcus/td/internal/serverdb"
)

// newTestServer creates a Server backed by temp directories for testing.
func newTestServer(t *testing.T) (*Server, *serverdb.ServerDB) {
	t.Helper()
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "server.db")
	store, err := serverdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	projectDir := filepath.Join(tmpDir, "projects")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}

	cfg := Config{
		ListenAddr:     ":0",
		ServerDBPath:   dbPath,
		ProjectDataDir: projectDir,
	}

	srv, err := NewServer(cfg, store)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	t.Cleanup(func() { srv.dbPool.CloseAll() })

	return srv, store
}

// createTestUser creates a user and API key, returning the bearer token.
func createTestUser(t *testing.T, store *serverdb.ServerDB, email string) (string, string) {
	t.Helper()
	user, err := store.CreateUser(email)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, _, err := store.GenerateAPIKey(user.ID, "test", "sync", nil)
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	return user.ID, token
}

func doRequest(srv *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}

	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)
	return w
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	w := doRequest(srv, "GET", "/healthz", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", resp["status"])
	}
}

func TestPushRequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	w := doRequest(srv, "POST", "/v1/projects/fake/sync/push", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPushSuccess(t *testing.T) {
	srv, store := newTestServer(t)
	userID, token := createTestUser(t, store, "push@test.com")

	// Create project
	w := doRequest(srv, "POST", "/v1/projects", token, CreateProjectRequest{
		Name: "test-project",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)
	_ = userID

	// Push events
	pushBody := PushRequest{
		DeviceID:  "dev1",
		SessionID: "sess1",
		Events: []EventInput{
			{
				ClientActionID:  1,
				ActionType:      "create",
				EntityType:      "issues",
				EntityID:        "i_001",
				Payload:         json.RawMessage(`{"title":"test"}`),
				ClientTimestamp: "2025-01-01T00:00:00Z",
			},
			{
				ClientActionID:  2,
				ActionType:      "update",
				EntityType:      "issues",
				EntityID:        "i_001",
				Payload:         json.RawMessage(`{"title":"updated"}`),
				ClientTimestamp: "2025-01-01T00:00:01Z",
			},
		},
	}

	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), token, pushBody)
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pushResp PushResponse
	json.NewDecoder(w.Body).Decode(&pushResp)

	if pushResp.Accepted != 2 {
		t.Fatalf("expected 2 accepted, got %d", pushResp.Accepted)
	}
	if len(pushResp.Acks) != 2 {
		t.Fatalf("expected 2 acks, got %d", len(pushResp.Acks))
	}
	if pushResp.Acks[0].ServerSeq < 1 {
		t.Fatalf("expected server_seq >= 1, got %d", pushResp.Acks[0].ServerSeq)
	}
}

func TestPullSuccess(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestUser(t, store, "pull@test.com")

	// Create project
	w := doRequest(srv, "POST", "/v1/projects", token, CreateProjectRequest{Name: "pull-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Push events
	pushBody := PushRequest{
		DeviceID:  "dev1",
		SessionID: "sess1",
		Events: []EventInput{
			{
				ClientActionID:  1,
				ActionType:      "create",
				EntityType:      "issues",
				EntityID:        "i_001",
				Payload:         json.RawMessage(`{"title":"test"}`),
				ClientTimestamp: "2025-01-01T00:00:00Z",
			},
		},
	}
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), token, pushBody)
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Pull events
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/pull?after_server_seq=0", project.ID), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("pull: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pullResp PullResponse
	json.NewDecoder(w.Body).Decode(&pullResp)

	if len(pullResp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pullResp.Events))
	}
	if pullResp.Events[0].EntityID != "i_001" {
		t.Fatalf("expected entity_id i_001, got %s", pullResp.Events[0].EntityID)
	}
	if pullResp.LastServerSeq < 1 {
		t.Fatalf("expected last_server_seq >= 1, got %d", pullResp.LastServerSeq)
	}
}

func TestPullPagination(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestUser(t, store, "page@test.com")

	// Create project
	w := doRequest(srv, "POST", "/v1/projects", token, CreateProjectRequest{Name: "page-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", w.Code)
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Push 5 events
	events := make([]EventInput, 5)
	for i := range events {
		events[i] = EventInput{
			ClientActionID:  int64(i + 1),
			ActionType:      "create",
			EntityType:      "issues",
			EntityID:        fmt.Sprintf("i_%03d", i+1),
			Payload:         json.RawMessage(`{}`),
			ClientTimestamp: "2025-01-01T00:00:00Z",
		}
	}

	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), token, PushRequest{
		DeviceID: "dev1", SessionID: "sess1", Events: events,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d", w.Code)
	}

	// Pull with limit=2
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/pull?after_server_seq=0&limit=2", project.ID), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("pull: expected 200, got %d", w.Code)
	}

	var pullResp PullResponse
	json.NewDecoder(w.Body).Decode(&pullResp)

	if len(pullResp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(pullResp.Events))
	}
	if !pullResp.HasMore {
		t.Fatal("expected has_more=true")
	}

	// Pull next page
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/pull?after_server_seq=%d&limit=2", project.ID, pullResp.LastServerSeq), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("pull page 2: expected 200, got %d", w.Code)
	}

	var pullResp2 PullResponse
	json.NewDecoder(w.Body).Decode(&pullResp2)

	if len(pullResp2.Events) != 2 {
		t.Fatalf("expected 2 events on page 2, got %d", len(pullResp2.Events))
	}
	if !pullResp2.HasMore {
		t.Fatal("expected has_more=true on page 2")
	}
}

func TestCreateProject(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestUser(t, store, "create@test.com")

	w := doRequest(srv, "POST", "/v1/projects", token, CreateProjectRequest{
		Name:        "my-project",
		Description: "a test project",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp ProjectResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Name != "my-project" {
		t.Fatalf("expected name my-project, got %s", resp.Name)
	}
	if resp.Description != "a test project" {
		t.Fatalf("expected description 'a test project', got %s", resp.Description)
	}
	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestListProjects(t *testing.T) {
	srv, store := newTestServer(t)
	userID1, token1 := createTestUser(t, store, "user1@test.com")
	_, token2 := createTestUser(t, store, "user2@test.com")
	_ = userID1

	// User 1 creates a project
	w := doRequest(srv, "POST", "/v1/projects", token1, CreateProjectRequest{Name: "user1-project"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// User 1 should see their project
	w = doRequest(srv, "GET", "/v1/projects", token1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var projects1 []ProjectResponse
	json.NewDecoder(w.Body).Decode(&projects1)
	if len(projects1) != 1 {
		t.Fatalf("expected 1 project for user1, got %d", len(projects1))
	}

	// User 2 should see no projects
	w = doRequest(srv, "GET", "/v1/projects", token2, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var projects2 []ProjectResponse
	json.NewDecoder(w.Body).Decode(&projects2)
	if len(projects2) != 0 {
		t.Fatalf("expected 0 projects for user2, got %d", len(projects2))
	}
}

func TestAddMember(t *testing.T) {
	srv, store := newTestServer(t)
	_, token1 := createTestUser(t, store, "owner@test.com")
	user2ID, _ := createTestUser(t, store, "member@test.com")

	// Create project
	w := doRequest(srv, "POST", "/v1/projects", token1, CreateProjectRequest{Name: "member-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Add member
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/members", project.ID), token1, AddMemberRequest{
		UserID: user2ID,
		Role:   "writer",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add member: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var memberResp MemberResponse
	json.NewDecoder(w.Body).Decode(&memberResp)
	if memberResp.Role != "writer" {
		t.Fatalf("expected role writer, got %s", memberResp.Role)
	}
}

func TestMemberRoleEnforcement(t *testing.T) {
	srv, store := newTestServer(t)
	_, token1 := createTestUser(t, store, "owner2@test.com")
	user2ID, token2 := createTestUser(t, store, "writer@test.com")
	user3ID, _ := createTestUser(t, store, "reader@test.com")

	// Owner creates project
	w := doRequest(srv, "POST", "/v1/projects", token1, CreateProjectRequest{Name: "role-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Add user2 as writer
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/members", project.ID), token1, AddMemberRequest{
		UserID: user2ID, Role: "writer",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add writer: expected 201, got %d", w.Code)
	}

	// Writer tries to add a member (should fail, needs owner)
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/members", project.ID), token2, AddMemberRequest{
		UserID: user3ID, Role: "reader",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("writer adding member: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncStatus(t *testing.T) {
	srv, store := newTestServer(t)
	_, token := createTestUser(t, store, "status@test.com")

	// Create project
	w := doRequest(srv, "POST", "/v1/projects", token, CreateProjectRequest{Name: "status-test"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}
	var project ProjectResponse
	json.NewDecoder(w.Body).Decode(&project)

	// Check status before any events
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/status", project.ID), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", w.Code)
	}
	var status SyncStatusResponse
	json.NewDecoder(w.Body).Decode(&status)
	if status.EventCount != 0 {
		t.Fatalf("expected 0 events, got %d", status.EventCount)
	}

	// Push 3 events
	w = doRequest(srv, "POST", fmt.Sprintf("/v1/projects/%s/sync/push", project.ID), token, PushRequest{
		DeviceID: "dev1", SessionID: "sess1",
		Events: []EventInput{
			{ClientActionID: 1, ActionType: "create", EntityType: "issues", EntityID: "i_001", Payload: json.RawMessage(`{}`), ClientTimestamp: "2025-01-01T00:00:00Z"},
			{ClientActionID: 2, ActionType: "create", EntityType: "logs", EntityID: "l_001", Payload: json.RawMessage(`{}`), ClientTimestamp: "2025-01-01T00:00:01Z"},
			{ClientActionID: 3, ActionType: "create", EntityType: "comments", EntityID: "c_001", Payload: json.RawMessage(`{}`), ClientTimestamp: "2025-01-01T00:00:02Z"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("push: expected 200, got %d", w.Code)
	}

	// Check status after push
	w = doRequest(srv, "GET", fmt.Sprintf("/v1/projects/%s/sync/status", project.ID), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", w.Code)
	}
	json.NewDecoder(w.Body).Decode(&status)
	if status.EventCount != 3 {
		t.Fatalf("expected 3 events, got %d", status.EventCount)
	}
	if status.LastServerSeq < 3 {
		t.Fatalf("expected last_server_seq >= 3, got %d", status.LastServerSeq)
	}
}
