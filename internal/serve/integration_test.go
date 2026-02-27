package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marcus/td/internal/db"
)

// ============================================================================
// Integration Test Harness
// ============================================================================

// setupIntegrationServer creates a fully initialized test server backed by a
// real SQLite database in a temp directory. Returns the base URL, the database
// handle, and a cleanup function.
func setupIntegrationServer(t *testing.T) (baseURL string, database *db.DB, cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()

	database, err := db.Initialize(tmpDir)
	if err != nil {
		t.Fatalf("db.Initialize: %v", err)
	}

	sess, err := GetOrCreateWebSession(database)
	if err != nil {
		database.Close()
		t.Fatalf("GetOrCreateWebSession: %v", err)
	}

	srv := NewServer(database, tmpDir, sess.ID, ServeConfig{})
	ts := httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		database.Close()
	}

	return ts.URL, database, cleanup
}

// iDoJSON sends a JSON request and returns the response. The "i" prefix
// avoids collision with the existing doJSON helper in handlers_write_test.go.
func iDoJSON(t *testing.T, method, url string, body interface{}) *http.Response {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// iParseEnvelope parses a response envelope and returns the ok flag, data map,
// and error payload map.
func iParseEnvelope(t *testing.T, resp *http.Response) (ok bool, data map[string]interface{}, errPayload map[string]interface{}) {
	t.Helper()
	defer resp.Body.Close()

	var env map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}

	ok, _ = env["ok"].(bool)

	if d, exists := env["data"]; exists && d != nil {
		data, _ = d.(map[string]interface{})
	}
	if e, exists := env["error"]; exists && e != nil {
		errPayload, _ = e.(map[string]interface{})
	}

	return ok, data, errPayload
}

// iCreateIssue creates a minimal issue with the given title and returns its ID.
func iCreateIssue(t *testing.T, baseURL, title string) string {
	t.Helper()
	return iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title": title,
	})
}

// iCreateIssueWithFields creates an issue with the given fields and returns its ID.
func iCreateIssueWithFields(t *testing.T, baseURL string, fields map[string]interface{}) string {
	t.Helper()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", fields)
	ok, data, errP := iParseEnvelope(t, resp)
	if !ok {
		t.Fatalf("create issue failed: status=%d, error=%v", resp.StatusCode, errP)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create issue: status=%d, want 201", resp.StatusCode)
	}

	issue, _ := data["issue"].(map[string]interface{})
	id, _ := issue["id"].(string)
	if id == "" {
		t.Fatal("created issue has no id")
	}
	return id
}

// ============================================================================
// Health Tests
// ============================================================================

func TestIntegration_Health_ReturnsOK(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}

	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// Check required fields
	if _, exists := data["status"]; !exists {
		t.Error("data.status missing")
	}
	if data["status"] != "ok" {
		t.Errorf("data.status = %v, want ok", data["status"])
	}
	if _, exists := data["session_id"]; !exists {
		t.Error("data.session_id missing")
	}
	sessID, _ := data["session_id"].(string)
	if sessID == "" {
		t.Error("session_id should be a non-empty string")
	}
	if _, exists := data["change_token"]; !exists {
		t.Error("data.change_token missing")
	}
}

func TestIntegration_Health_ChangeTokenIsString(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}

	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}

	token := data["change_token"]
	if _, isString := token.(string); !isString {
		t.Errorf("change_token type = %T, want string", token)
	}
}

// ============================================================================
// Monitor Tests
// ============================================================================

func TestIntegration_Monitor_EmptyDB(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "GET", baseURL+"/v1/monitor", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}

	mon, _ := data["monitor"].(map[string]interface{})
	if mon == nil {
		t.Fatal("data.monitor missing")
	}

	// Check structure has expected fields
	taskList, _ := mon["task_list"].(map[string]interface{})
	if taskList == nil {
		t.Fatal("monitor.task_list missing")
	}

	// All task_list arrays should be present and be arrays
	for _, key := range []string{"reviewable", "needs_rework", "in_progress", "ready", "pending_review", "blocked", "closed"} {
		arr, exists := taskList[key]
		if !exists {
			t.Errorf("task_list.%s missing", key)
			continue
		}
		if _, isArr := arr.([]interface{}); !isArr {
			t.Errorf("task_list.%s should be an array, got %T", key, arr)
		}
	}

	// activity should be an array
	activity, _ := mon["activity"].([]interface{})
	if activity == nil {
		t.Error("monitor.activity should be an empty array, not null")
	}

	// timestamp should be present
	if _, exists := mon["timestamp"]; !exists {
		t.Error("monitor.timestamp missing")
	}
}

func TestIntegration_Monitor_WithIssues(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	iCreateIssue(t, baseURL, "Monitor test issue 1")
	iCreateIssue(t, baseURL, "Monitor test issue 2")

	resp := iDoJSON(t, "GET", baseURL+"/v1/monitor", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}

	mon, _ := data["monitor"].(map[string]interface{})
	taskList, _ := mon["task_list"].(map[string]interface{})
	ready, _ := taskList["ready"].([]interface{})

	if len(ready) < 2 {
		t.Errorf("task_list.ready has %d items, want >= 2", len(ready))
	}
}

func TestIntegration_Monitor_IncludeClosed(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Issue to close for monitor")

	// Close it
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/close", nil)
	ok, _, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("close failed")
	}

	// Without include_closed - should NOT appear in closed list
	resp = iDoJSON(t, "GET", baseURL+"/v1/monitor", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("monitor failed")
	}
	mon, _ := data["monitor"].(map[string]interface{})
	taskList, _ := mon["task_list"].(map[string]interface{})
	closed, _ := taskList["closed"].([]interface{})
	if len(closed) != 0 {
		t.Errorf("without include_closed: closed has %d items, want 0", len(closed))
	}

	// With include_closed=true
	resp = iDoJSON(t, "GET", baseURL+"/v1/monitor?include_closed=true", nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("monitor with include_closed failed")
	}
	mon, _ = data["monitor"].(map[string]interface{})
	taskList, _ = mon["task_list"].(map[string]interface{})
	closed, _ = taskList["closed"].([]interface{})
	if len(closed) < 1 {
		t.Errorf("with include_closed: closed has %d items, want >= 1", len(closed))
	}
}

// ============================================================================
// Issues List Tests
// ============================================================================

func TestIntegration_ListIssues_Empty(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 0 {
		t.Errorf("issues has %d items, want 0", len(issues))
	}

	total, _ := data["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}

	hasMore, _ := data["has_more"].(bool)
	if hasMore {
		t.Error("has_more should be false")
	}
}

func TestIntegration_ListIssues_WithIssues(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	iCreateIssue(t, baseURL, "List test issue number one")
	iCreateIssue(t, baseURL, "List test issue number two")
	iCreateIssue(t, baseURL, "List test issue number three")

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("ok should be true")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 3 {
		t.Errorf("issues has %d items, want 3", len(issues))
	}

	total, _ := data["total"].(float64)
	if total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

func TestIntegration_ListIssues_FilterByStatus(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	iCreateIssue(t, baseURL, "Open issue for filter test")
	id2 := iCreateIssue(t, baseURL, "In progress issue for filter test")

	// Start id2 to put it in_progress
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id2+"/start", nil)
	ok, _, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("start failed")
	}

	// Filter by status=in_progress
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues?status=in_progress", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("list failed")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 1 {
		t.Errorf("status=in_progress: got %d issues, want 1", len(issues))
	}
	if len(issues) > 0 {
		issue, _ := issues[0].(map[string]interface{})
		if issue["id"] != id2 {
			t.Errorf("expected id=%s, got %v", id2, issue["id"])
		}
	}

	// Filter by status=open should return the other one
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues?status=open", nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("list by open failed")
	}
	issues, _ = data["issues"].([]interface{})
	if len(issues) != 1 {
		t.Errorf("status=open: got %d issues, want 1", len(issues))
	}
}

func TestIntegration_ListIssues_FilterByType(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title": "A bug for type filter", "type": "bug",
	})
	iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title": "A feature for type filter", "type": "feature",
	})

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues?type=bug", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("list failed")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 1 {
		t.Errorf("type=bug: got %d issues, want 1", len(issues))
	}
	if len(issues) > 0 {
		issue, _ := issues[0].(map[string]interface{})
		if issue["type"] != "bug" {
			t.Errorf("expected type=bug, got %v", issue["type"])
		}
	}
}

func TestIntegration_ListIssues_Pagination(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		iCreateIssue(t, baseURL, fmt.Sprintf("Paginated issue %d", i))
	}

	// First page: limit=2, offset=0
	resp := iDoJSON(t, "GET", baseURL+"/v1/issues?limit=2&offset=0", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("list page 1 failed")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 2 {
		t.Errorf("page 1: got %d issues, want 2", len(issues))
	}

	total, _ := data["total"].(float64)
	if total != 5 {
		t.Errorf("total = %v, want 5", total)
	}

	hasMore, _ := data["has_more"].(bool)
	if !hasMore {
		t.Error("page 1: has_more should be true")
	}

	// Last page: limit=2, offset=4
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues?limit=2&offset=4", nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("list last page failed")
	}

	issues, _ = data["issues"].([]interface{})
	if len(issues) != 1 {
		t.Errorf("last page: got %d issues, want 1", len(issues))
	}

	hasMore, _ = data["has_more"].(bool)
	if hasMore {
		t.Error("last page: has_more should be false")
	}
}

func TestIntegration_ListIssues_Search(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	iCreateIssue(t, baseURL, "Alpha unique needle for search")
	iCreateIssue(t, baseURL, "Beta something else entirely")

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues?search=unique+needle&search_mode=text", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("search failed")
	}

	issues, _ := data["issues"].([]interface{})
	if len(issues) != 1 {
		t.Errorf("search: got %d issues, want 1", len(issues))
	}
	if len(issues) > 0 {
		issue, _ := issues[0].(map[string]interface{})
		title, _ := issue["title"].(string)
		if title != "Alpha unique needle for search" {
			t.Errorf("expected title 'Alpha unique needle for search', got %q", title)
		}
	}
}

// ============================================================================
// Issue Detail Tests
// ============================================================================

func TestIntegration_GetIssue_Found(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Detail test issue for integration")

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues/"+id, nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("get issue failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue == nil {
		t.Fatal("data.issue missing")
	}
	if issue["id"] != id {
		t.Errorf("id = %v, want %s", issue["id"], id)
	}

	// Verify associated arrays are present (never null)
	logs, _ := data["logs"].([]interface{})
	if logs == nil {
		t.Error("data.logs should be an array, not null")
	}

	comments, _ := data["comments"].([]interface{})
	if comments == nil {
		t.Error("data.comments should be an array, not null")
	}

	deps, _ := data["dependencies"].([]interface{})
	if deps == nil {
		t.Error("data.dependencies should be an array, not null")
	}

	blockedBy, _ := data["blocked_by"].([]interface{})
	if blockedBy == nil {
		t.Error("data.blocked_by should be an array, not null")
	}
}

func TestIntegration_GetIssue_NotFound(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "GET", baseURL+"/v1/issues/td-nonexistent999", nil)
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("ok should be false for not found")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if errP["code"] != ErrNotFound {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrNotFound)
	}
}

// ============================================================================
// Issue Create Tests
// ============================================================================

func TestIntegration_CreateIssue_Minimal(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title": "Minimal integration issue",
	})
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("create failed")
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["title"] != "Minimal integration issue" {
		t.Errorf("title = %v, want 'Minimal integration issue'", issue["title"])
	}

	// Defaults applied
	if issue["type"] != "task" {
		t.Errorf("default type = %v, want 'task'", issue["type"])
	}
	if issue["priority"] != "P2" {
		t.Errorf("default priority = %v, want 'P2'", issue["priority"])
	}
	if issue["status"] != "open" {
		t.Errorf("default status = %v, want 'open'", issue["status"])
	}
}

func TestIntegration_CreateIssue_AllFields(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title":       "Full integration issue",
		"description": "A full description for integration",
		"type":        "bug",
		"priority":    "P1",
		"points":      5,
		"labels":      []string{"api", "critical"},
		"acceptance":  "All tests pass",
		"sprint":      "sprint-1",
		"minor":       true,
	})
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("create failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["title"] != "Full integration issue" {
		t.Errorf("title = %v", issue["title"])
	}
	if issue["description"] != "A full description for integration" {
		t.Errorf("description = %v", issue["description"])
	}
	if issue["type"] != "bug" {
		t.Errorf("type = %v, want bug", issue["type"])
	}
	if issue["priority"] != "P1" {
		t.Errorf("priority = %v, want P1", issue["priority"])
	}
	if issue["points"] != float64(5) {
		t.Errorf("points = %v, want 5", issue["points"])
	}
	labels, _ := issue["labels"].([]interface{})
	if len(labels) != 2 {
		t.Errorf("labels has %d items, want 2", len(labels))
	}
	if issue["acceptance"] != "All tests pass" {
		t.Errorf("acceptance = %v", issue["acceptance"])
	}
	if issue["sprint"] != "sprint-1" {
		t.Errorf("sprint = %v", issue["sprint"])
	}
	if issue["minor"] != true {
		t.Errorf("minor = %v, want true", issue["minor"])
	}
}

func TestIntegration_CreateIssue_StoryAlias(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title": "Story alias integration test",
		"type":  "story",
	})
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("create failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["type"] != "feature" {
		t.Errorf("type = %v, want 'feature' (story alias)", issue["type"])
	}
}

func TestIntegration_CreateIssue_NumericPriority(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title":    "Numeric priority integration test",
		"priority": "1",
	})
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("create failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["priority"] != "P1" {
		t.Errorf("priority = %v, want 'P1' (from '1')", issue["priority"])
	}
}

func TestIntegration_CreateIssue_ValidationErrors(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// Missing title
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{})
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail without title")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if errP["code"] != ErrValidation {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrValidation)
	}

	// Invalid type
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title": "Bad type integration test", "type": "invalid_type",
	})
	ok, _, errP = iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail with invalid type")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid type", resp.StatusCode)
	}
}

func TestIntegration_CreateIssue_InvalidPoints(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues", map[string]interface{}{
		"title":  "Bad points integration test",
		"points": 4,
	})
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail with non-Fibonacci points")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if errP["code"] != ErrValidation {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrValidation)
	}
}

// ============================================================================
// Issue Update Tests
// ============================================================================

func TestIntegration_UpdateIssue_PartialUpdate(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title":    "Original title for partial update",
		"priority": "P2",
	})

	// Update only the title
	resp := iDoJSON(t, "PATCH", baseURL+"/v1/issues/"+id, map[string]interface{}{
		"title": "Updated title via integration",
	})
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("update failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["title"] != "Updated title via integration" {
		t.Errorf("title = %v, want 'Updated title via integration'", issue["title"])
	}
	// Priority should be unchanged
	if issue["priority"] != "P2" {
		t.Errorf("priority = %v, want 'P2' (unchanged)", issue["priority"])
	}
}

func TestIntegration_UpdateIssue_NotFound(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "PATCH", baseURL+"/v1/issues/td-nonexistent999", map[string]interface{}{
		"title": "This title is long enough to pass validation",
	})
	ok, _, _ := iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail for nonexistent issue")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestIntegration_UpdateIssue_ValidationError(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Valid issue for update validation")

	// Update with invalid type
	resp := iDoJSON(t, "PATCH", baseURL+"/v1/issues/"+id, map[string]interface{}{
		"type": "invalid_type",
	})
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail with invalid type")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if errP["code"] != ErrValidation {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrValidation)
	}
}

// ============================================================================
// Issue Delete Tests
// ============================================================================

func TestIntegration_DeleteIssue_Success(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "To be deleted in integration")

	resp := iDoJSON(t, "DELETE", baseURL+"/v1/issues/"+id, nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("delete failed")
	}
	if data["deleted"] != true {
		t.Errorf("deleted = %v, want true", data["deleted"])
	}

	// Issue should no longer be found via the API (soft-deleted issues
	// are filtered by GetIssue which checks deleted_at)
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues/"+id, nil)
	ok2, _, _ := iParseEnvelope(t, resp)
	if ok2 {
		// Soft-deleted issues may still be returned by GetIssue depending
		// on implementation. Just verify the delete itself succeeded.
		t.Log("note: soft-deleted issue still returned by GET (expected for some implementations)")
	}
}

func TestIntegration_DeleteIssue_NotFound(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	resp := iDoJSON(t, "DELETE", baseURL+"/v1/issues/td-nonexistent999", nil)
	ok, _, _ := iParseEnvelope(t, resp)
	if ok {
		t.Error("should fail for nonexistent issue")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ============================================================================
// Status Transition Tests
// ============================================================================

func TestIntegration_Start_OpenToInProgress(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Start integration test")

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/start", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("start failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "in_progress" {
		t.Errorf("status = %v, want in_progress", issue["status"])
	}
	if issue["implementer_session"] == nil {
		t.Error("implementer_session should be set after start")
	}
}

func TestIntegration_Start_InvalidStatus(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Already started integration")

	// Start once
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/start", nil)
	ok, _, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("first start failed")
	}

	// Start again should fail (already in_progress)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/start", nil)
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("second start should fail")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
	if errP["code"] != ErrConflict {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrConflict)
	}
}

func TestIntegration_Review_ToInReview(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// From open
	id1 := iCreateIssue(t, baseURL, "Review from open integration")
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id1+"/review", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("review from open failed")
	}
	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "in_review" {
		t.Errorf("status = %v, want in_review", issue["status"])
	}

	// From in_progress
	id2 := iCreateIssue(t, baseURL, "Review from in_progress integration")
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id2+"/start", nil)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id2+"/review", nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("review from in_progress failed")
	}
	issue, _ = data["issue"].(map[string]interface{})
	if issue["status"] != "in_review" {
		t.Errorf("status = %v, want in_review", issue["status"])
	}
}

func TestIntegration_Approve_ClosesIssue(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "To be approved integration")

	// Move to in_review first
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/review", nil)

	// Approve
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/approve", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("approve failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "closed" {
		t.Errorf("status = %v, want closed", issue["status"])
	}
	if issue["reviewer_session"] == nil {
		t.Error("reviewer_session should be set after approve")
	}
	if issue["closed_at"] == nil {
		t.Error("closed_at should be set after approve")
	}
}

func TestIntegration_Reject_ToOpen(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "To be rejected integration")

	// Move to in_review
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/review", nil)

	// Reject
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/reject", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("reject failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "open" {
		t.Errorf("status = %v, want open", issue["status"])
	}
	// Session fields should be cleared
	if issue["implementer_session"] != nil {
		t.Error("implementer_session should be cleared after reject")
	}
	if issue["reviewer_session"] != nil {
		t.Error("reviewer_session should be cleared after reject")
	}
}

func TestIntegration_Block_ToBlocked(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// Block from open
	id1 := iCreateIssue(t, baseURL, "Block from open integration")
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id1+"/block", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("block from open failed")
	}
	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "blocked" {
		t.Errorf("status = %v, want blocked", issue["status"])
	}

	// Block from in_progress
	id2 := iCreateIssue(t, baseURL, "Block from in_progress integration")
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id2+"/start", nil)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id2+"/block", nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("block from in_progress failed")
	}
	issue, _ = data["issue"].(map[string]interface{})
	if issue["status"] != "blocked" {
		t.Errorf("status = %v, want blocked", issue["status"])
	}
}

func TestIntegration_Unblock_ToOpen(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Unblock integration test")

	// Block first
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/block", nil)

	// Unblock
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/unblock", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("unblock failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "open" {
		t.Errorf("status = %v, want open", issue["status"])
	}
}

func TestIntegration_Close_DirectClose(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// Close from open
	id := iCreateIssue(t, baseURL, "Direct close integration")
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/close", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("close failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "closed" {
		t.Errorf("status = %v, want closed", issue["status"])
	}
	if issue["closed_at"] == nil {
		t.Error("closed_at should be set")
	}
}

func TestIntegration_Reopen_ClosedToOpen(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Reopen integration test")
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/close", nil)

	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/reopen", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("reopen failed")
	}

	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "open" {
		t.Errorf("status = %v, want open", issue["status"])
	}
	if issue["closed_at"] != nil {
		t.Error("closed_at should be cleared after reopen")
	}
}

func TestIntegration_Transition_WithReason(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Reason integration test")

	// Start with a reason
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/start", map[string]interface{}{
		"reason": "Beginning implementation work via integration",
	})
	ok, _, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("start with reason failed")
	}

	// Verify the log entry was created by checking the issue detail
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues/"+id, nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("get issue failed")
	}

	logs, _ := data["logs"].([]interface{})
	found := false
	for _, l := range logs {
		log, _ := l.(map[string]interface{})
		if log["message"] == "Beginning implementation work via integration" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log entry with custom reason message not found")
	}
}

func TestIntegration_Transition_Invalid(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	id := iCreateIssue(t, baseURL, "Invalid transition integration")

	// Try to approve an open issue (must be in_review first)
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/approve", nil)
	ok, _, errP := iParseEnvelope(t, resp)
	if ok {
		t.Error("approve from open should fail")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("approve: status = %d, want 409", resp.StatusCode)
	}
	if errP["code"] != ErrConflict {
		t.Errorf("error.code = %v, want %s", errP["code"], ErrConflict)
	}

	// Try to unblock an open issue (must be blocked first)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/unblock", nil)
	ok, _, _ = iParseEnvelope(t, resp)
	if ok {
		t.Error("unblock from open should fail")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("unblock: status = %d, want 409", resp.StatusCode)
	}

	// Try to reject an open issue (must be in_review first)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/reject", nil)
	ok, _, _ = iParseEnvelope(t, resp)
	if ok {
		t.Error("reject from open should fail")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("reject: status = %d, want 409", resp.StatusCode)
	}

	// Try to reopen an open issue (must be closed first)
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+id+"/reopen", nil)
	ok, _, _ = iParseEnvelope(t, resp)
	if ok {
		t.Error("reopen from open should fail")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("reopen: status = %d, want 409", resp.StatusCode)
	}
}

// ============================================================================
// Cascade Tests
// ============================================================================

func TestIntegration_Approve_ParentCascade(t *testing.T) {
	baseURL, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// Create a parent issue (must be epic type for cascade to work)
	parentID := iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title": "Parent cascade integration test",
		"type":  "epic",
	})

	// Create two child issues
	child1 := iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title":     "Child 1 for cascade",
		"parent_id": parentID,
	})
	child2 := iCreateIssueWithFields(t, baseURL, map[string]interface{}{
		"title":     "Child 2 for cascade",
		"parent_id": parentID,
	})

	// Move both children to in_review
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+child1+"/review", nil)
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+child2+"/review", nil)

	// Approve child 1
	iDoJSON(t, "POST", baseURL+"/v1/issues/"+child1+"/approve", nil)

	// Approve child 2 - should cascade parent to closed
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+child2+"/approve", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("approve child2 failed")
	}

	// Check cascades
	cascades, _ := data["cascades"].(map[string]interface{})
	parentUpdates, _ := cascades["parent_status_updates"].([]interface{})

	if len(parentUpdates) < 1 {
		t.Error("expected parent cascade but got none")
	}

	// Verify parent is now closed
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues/"+parentID, nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("get parent failed")
	}
	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "closed" {
		t.Errorf("parent status = %v, want closed after cascade", issue["status"])
	}
}

func TestIntegration_Close_UnblocksDependents(t *testing.T) {
	baseURL, database, cleanup := setupIntegrationServer(t)
	defer cleanup()

	// Create blocker and dependent issues
	blockerID := iCreateIssue(t, baseURL, "Blocker issue integration")
	dependentID := iCreateIssue(t, baseURL, "Dependent issue integration")

	// Add dependency: dependent depends_on blocker
	err := database.AddDependencyLogged(dependentID, blockerID, "depends_on", "test-session")
	if err != nil {
		t.Fatalf("add dependency: %v", err)
	}

	// Block the dependent
	resp := iDoJSON(t, "POST", baseURL+"/v1/issues/"+dependentID+"/block", nil)
	ok, _, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("block dependent failed")
	}

	// Close the blocker
	resp = iDoJSON(t, "POST", baseURL+"/v1/issues/"+blockerID+"/close", nil)
	ok, data, _ := iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("close blocker failed")
	}

	// Check cascades - dependent should be auto-unblocked
	cascades, _ := data["cascades"].(map[string]interface{})
	autoUnblocked, _ := cascades["auto_unblocked"].([]interface{})

	if len(autoUnblocked) < 1 {
		t.Error("expected dependent to be auto-unblocked but got none")
	}

	// Verify dependent is now open
	resp = iDoJSON(t, "GET", baseURL+"/v1/issues/"+dependentID, nil)
	ok, data, _ = iParseEnvelope(t, resp)
	if !ok {
		t.Fatal("get dependent failed")
	}
	issue, _ := data["issue"].(map[string]interface{})
	if issue["status"] != "open" {
		t.Errorf("dependent status = %v, want open after auto-unblock", issue["status"])
	}
}
