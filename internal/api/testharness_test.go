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

// TestHarness wraps a full Server with a real HTTP listener for integration tests.
type TestHarness struct {
	t       *testing.T
	Server  *Server
	Store   *serverdb.ServerDB
	BaseURL string
	client  *http.Client
	httpSrv *httptest.Server
}

// newTestHarness creates a TestHarness with a real HTTP server on a random port.
func newTestHarness(t *testing.T, opts ...func(*Config)) *TestHarness {
	t.Helper()

	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "server.db")
	store, err := serverdb.Open(dbPath)
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}

	projectDir := filepath.Join(tmpDir, "projects")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}

	cfg := Config{
		RateLimitAuth:  100000,
		RateLimitPush:  100000,
		RateLimitPull:  100000,
		RateLimitOther: 100000,
		ListenAddr:     ":0",
		ServerDBPath:   dbPath,
		ProjectDataDir: projectDir,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	srv, err := NewServer(cfg, store)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	httpSrv := httptest.NewServer(srv.routes())

	h := &TestHarness{
		t:       t,
		Server:  srv,
		Store:   store,
		BaseURL: httpSrv.URL,
		client:  &http.Client{},
		httpSrv: httpSrv,
	}

	t.Cleanup(func() {
		httpSrv.Close()
		srv.dbPool.CloseAll()
		store.Close()
	})

	return h
}

// Do sends an HTTP request and returns the response.
func (h *TestHarness) Do(method, path, token string, body any) *http.Response {
	h.t.Helper()

	url := h.BaseURL + path

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("marshal request body: %v", err)
		}
	}

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, &buf)
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		h.t.Fatalf("create request: %v", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatalf("do request %s %s: %v", method, path, err)
	}

	return resp
}

// DoJSON sends an HTTP request and decodes the JSON response into out.
func (h *TestHarness) DoJSON(method, path, token string, body any, out any) *http.Response {
	h.t.Helper()

	resp := h.Do(method, path, token, body)

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		h.t.Fatalf("decode response: %v", err)
	}

	return resp
}

// CreateUser creates a user with a sync-scoped API key.
func (h *TestHarness) CreateUser(email string) (userID, token string) {
	h.t.Helper()

	user, err := h.Store.CreateUser(email)
	if err != nil {
		h.t.Fatalf("create user: %v", err)
	}

	tok, _, err := h.Store.GenerateAPIKey(user.ID, "test", "sync", nil)
	if err != nil {
		h.t.Fatalf("generate api key: %v", err)
	}

	return user.ID, tok
}

// CreateAdminUser creates an admin user with specified scopes.
func (h *TestHarness) CreateAdminUser(email, scopes string) (userID, token string) {
	h.t.Helper()

	user, err := h.Store.CreateUser(email)
	if err != nil {
		h.t.Fatalf("create user: %v", err)
	}

	if !user.IsAdmin {
		if err := h.Store.SetUserAdmin(email, true); err != nil {
			h.t.Fatalf("set admin: %v", err)
		}
	}

	tok, _, err := h.Store.GenerateAPIKey(user.ID, "admin-test", scopes, nil)
	if err != nil {
		h.t.Fatalf("generate api key: %v", err)
	}

	return user.ID, tok
}

// CreateProject creates a project via the API. Returns project ID.
func (h *TestHarness) CreateProject(ownerToken, name string) string {
	h.t.Helper()

	var project ProjectResponse
	resp := h.DoJSON("POST", "/v1/projects", ownerToken, CreateProjectRequest{Name: name}, &project)

	if resp.StatusCode != http.StatusCreated {
		h.t.Fatalf("create project: expected 201, got %d", resp.StatusCode)
	}

	return project.ID
}

// PushEvents pushes events to a project via the API.
func (h *TestHarness) PushEvents(token, projectID string, events []EventInput) {
	h.t.Helper()

	resp := h.Do("POST", fmt.Sprintf("/v1/projects/%s/sync/push", projectID), token, PushRequest{
		DeviceID:  "test-device",
		SessionID: "test-session",
		Events:    events,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("push events: expected 200, got %d", resp.StatusCode)
	}
}

// BuildSnapshot triggers a snapshot build by calling GET /v1/projects/{id}/sync/snapshot.
func (h *TestHarness) BuildSnapshot(token, projectID string) {
	h.t.Helper()

	resp := h.Do("GET", fmt.Sprintf("/v1/projects/%s/sync/snapshot", projectID), token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("build snapshot: expected 200, got %d", resp.StatusCode)
	}
}
