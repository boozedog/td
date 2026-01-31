package sync

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const clientTestSchema = `
CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    title TEXT,
    status TEXT,
    priority TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
CREATE TABLE action_log (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    previous_data TEXT DEFAULT '',
    new_data TEXT DEFAULT '',
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    undone INTEGER DEFAULT 0,
    synced_at DATETIME,
    server_seq INTEGER
);
`

func setupClientDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(clientTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertActionLog(t *testing.T, db *sql.DB, id, sessionID, actionType, entityType, entityID, newData, prevData string, undone int, syncedAt string) {
	t.Helper()
	var syncedAtVal any
	if syncedAt != "" {
		syncedAtVal = syncedAt
	}
	_, err := db.Exec(
		`INSERT INTO action_log (id, session_id, action_type, entity_type, entity_id, new_data, previous_data, timestamp, undone, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, actionType, entityType, entityID, newData, prevData,
		time.Now().UTC().Format("2006-01-02 15:04:05"), undone, syncedAtVal,
	)
	if err != nil {
		t.Fatalf("insert action_log: %v", err)
	}
}

func TestGetPendingEvents_Basic(t *testing.T) {
	db := setupClientDB(t)

	insertActionLog(t, db, "al-00000001", "sess1", "create", "issues", "i1",
		`{"title":"First","status":"open"}`, `{}`, 0, "")
	insertActionLog(t, db, "al-00000002", "sess1", "update", "issues", "i1",
		`{"title":"Updated","status":"open"}`, `{"title":"First","status":"open"}`, 0, "")
	insertActionLog(t, db, "al-00000003", "sess1", "delete", "issues", "i2",
		`{}`, `{"title":"Gone"}`, 0, "")

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	events, err := GetPendingEvents(tx, "device1", "sync-sess")
	if err != nil {
		t.Fatalf("GetPendingEvents: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}

	// Verify first event
	ev := events[0]
	if ev.DeviceID != "device1" {
		t.Errorf("DeviceID: got %q, want device1", ev.DeviceID)
	}
	if ev.SessionID != "sync-sess" {
		t.Errorf("SessionID: got %q, want sync-sess", ev.SessionID)
	}
	if ev.ActionType != "create" {
		t.Errorf("ActionType: got %q, want create", ev.ActionType)
	}
	if ev.EntityType != "issues" {
		t.Errorf("EntityType: got %q, want issues", ev.EntityType)
	}
	if ev.EntityID != "i1" {
		t.Errorf("EntityID: got %q, want i1", ev.EntityID)
	}
	if ev.ServerSeq != 0 {
		t.Errorf("ServerSeq: got %d, want 0", ev.ServerSeq)
	}
	if ev.ClientActionID <= 0 {
		t.Errorf("ClientActionID should be positive rowid, got %d", ev.ClientActionID)
	}

	// Verify payload structure
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := payload["schema_version"]; !ok {
		t.Error("payload missing schema_version")
	}
	if _, ok := payload["new_data"]; !ok {
		t.Error("payload missing new_data")
	}
	if _, ok := payload["previous_data"]; !ok {
		t.Error("payload missing previous_data")
	}

	// Verify ordering (rowids ascending)
	for i := 1; i < len(events); i++ {
		if events[i].ClientActionID <= events[i-1].ClientActionID {
			t.Errorf("events not ordered: [%d].rowid=%d <= [%d].rowid=%d",
				i, events[i].ClientActionID, i-1, events[i-1].ClientActionID)
		}
	}
}

func TestGetPendingEvents_SkipsUndone(t *testing.T) {
	db := setupClientDB(t)

	insertActionLog(t, db, "al-00000001", "sess1", "create", "issues", "i1",
		`{"title":"Keep"}`, `{}`, 0, "")
	insertActionLog(t, db, "al-00000002", "sess1", "create", "issues", "i2",
		`{"title":"Undone"}`, `{}`, 1, "")
	insertActionLog(t, db, "al-00000003", "sess1", "update", "issues", "i1",
		`{"title":"Also keep"}`, `{"title":"Keep"}`, 0, "")

	tx, _ := db.Begin()
	defer tx.Rollback()

	events, err := GetPendingEvents(tx, "d1", "s1")
	if err != nil {
		t.Fatalf("GetPendingEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].EntityID != "i1" || events[1].EntityID != "i1" {
		t.Errorf("expected both events for i1, got %q and %q", events[0].EntityID, events[1].EntityID)
	}
}

func TestGetPendingEvents_SkipsSynced(t *testing.T) {
	db := setupClientDB(t)

	insertActionLog(t, db, "al-00000001", "sess1", "create", "issues", "i1",
		`{"title":"Synced"}`, `{}`, 0, "2025-01-01 00:00:00")
	insertActionLog(t, db, "al-00000002", "sess1", "create", "issues", "i2",
		`{"title":"Pending"}`, `{}`, 0, "")

	tx, _ := db.Begin()
	defer tx.Rollback()

	events, err := GetPendingEvents(tx, "d1", "s1")
	if err != nil {
		t.Fatalf("GetPendingEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].EntityID != "i2" {
		t.Errorf("expected entity i2, got %q", events[0].EntityID)
	}
}

func TestGetPendingEvents_ActionTypeMapping(t *testing.T) {
	db := setupClientDB(t)

	cases := []struct {
		id       string
		tdAction string
		want     string
	}{
		{"al-00000001", "create", "create"},
		{"al-00000002", "update", "update"},
		{"al-00000003", "start", "update"},
		{"al-00000004", "delete", "delete"},
		{"al-00000005", "review", "update"},
		{"al-00000006", "approve", "update"},
		{"al-00000007", "close", "update"},
		{"al-00000008", "reopen", "update"},
	}

	for _, tc := range cases {
		insertActionLog(t, db, tc.id, "sess1", tc.tdAction, "issues", "i1",
			`{"title":"test"}`, `{}`, 0, "")
	}

	tx, _ := db.Begin()
	defer tx.Rollback()

	events, err := GetPendingEvents(tx, "d1", "s1")
	if err != nil {
		t.Fatalf("GetPendingEvents: %v", err)
	}
	if len(events) != len(cases) {
		t.Fatalf("got %d events, want %d", len(events), len(cases))
	}

	for i, tc := range cases {
		if events[i].ActionType != tc.want {
			t.Errorf("action %q: got %q, want %q", tc.tdAction, events[i].ActionType, tc.want)
		}
	}
}

func TestApplyRemoteEvents_Basic(t *testing.T) {
	db := setupClientDB(t)

	events := []Event{
		{
			ServerSeq:  1,
			ActionType: "create",
			EntityType: "issues",
			EntityID:   "i1",
			Payload:    []byte(`{"schema_version":1,"new_data":{"title":"First","status":"open"},"previous_data":{}}`),
		},
		{
			ServerSeq:  2,
			ActionType: "create",
			EntityType: "issues",
			EntityID:   "i2",
			Payload:    []byte(`{"schema_version":1,"new_data":{"title":"Second","status":"open"},"previous_data":{}}`),
		},
		{
			ServerSeq:  3,
			ActionType: "update",
			EntityType: "issues",
			EntityID:   "i1",
			Payload:    []byte(`{"schema_version":1,"new_data":{"title":"Updated First","status":"closed"},"previous_data":{"title":"First","status":"open"}}`),
		},
	}

	tx, _ := db.Begin()
	result, err := ApplyRemoteEvents(tx, events, "my-device", testValidator)
	if err != nil {
		t.Fatalf("ApplyRemoteEvents: %v", err)
	}
	tx.Commit()

	if result.Applied != 3 {
		t.Fatalf("Applied: got %d, want 3", result.Applied)
	}
	if result.LastAppliedSeq != 3 {
		t.Fatalf("LastAppliedSeq: got %d, want 3", result.LastAppliedSeq)
	}
	if len(result.Failed) != 0 {
		t.Fatalf("Failed: got %d, want 0", len(result.Failed))
	}

	// Verify entities in DB
	var title, status string
	if err := db.QueryRow("SELECT title, status FROM issues WHERE id = ?", "i1").Scan(&title, &status); err != nil {
		t.Fatalf("query i1: %v", err)
	}
	if title != "Updated First" || status != "closed" {
		t.Errorf("i1: title=%q status=%q", title, status)
	}

	if err := db.QueryRow("SELECT title, status FROM issues WHERE id = ?", "i2").Scan(&title, &status); err != nil {
		t.Fatalf("query i2: %v", err)
	}
	if title != "Second" || status != "open" {
		t.Errorf("i2: title=%q status=%q", title, status)
	}
}

func TestApplyRemoteEvents_PartialFailure(t *testing.T) {
	db := setupClientDB(t)

	events := []Event{
		{
			ServerSeq:  1,
			ActionType: "create",
			EntityType: "issues",
			EntityID:   "i1",
			Payload:    []byte(`{"schema_version":1,"new_data":{"title":"Good","status":"open"},"previous_data":{}}`),
		},
		{
			ServerSeq:  2,
			ActionType: "create",
			EntityType: "nonexistent_table",
			EntityID:   "x1",
			Payload:    []byte(`{"schema_version":1,"new_data":{"name":"Bad"},"previous_data":{}}`),
		},
		{
			ServerSeq:  3,
			ActionType: "create",
			EntityType: "issues",
			EntityID:   "i2",
			Payload:    []byte(`{"schema_version":1,"new_data":{"title":"Also Good","status":"open"},"previous_data":{}}`),
		},
	}

	tx, _ := db.Begin()
	result, err := ApplyRemoteEvents(tx, events, "my-device", testValidator)
	if err != nil {
		t.Fatalf("ApplyRemoteEvents: %v", err)
	}
	tx.Commit()

	if result.Applied != 2 {
		t.Fatalf("Applied: got %d, want 2", result.Applied)
	}
	if result.LastAppliedSeq != 3 {
		t.Fatalf("LastAppliedSeq: got %d, want 3", result.LastAppliedSeq)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("Failed: got %d, want 1", len(result.Failed))
	}
	if result.Failed[0].ServerSeq != 2 {
		t.Errorf("Failed[0].ServerSeq: got %d, want 2", result.Failed[0].ServerSeq)
	}

	// Verify good entities exist
	var count int
	db.QueryRow("SELECT COUNT(*) FROM issues").Scan(&count)
	if count != 2 {
		t.Fatalf("issues count: got %d, want 2", count)
	}
}

func TestMarkEventsSynced(t *testing.T) {
	db := setupClientDB(t)

	insertActionLog(t, db, "al-00000001", "sess1", "create", "issues", "i1",
		`{"title":"One"}`, `{}`, 0, "")
	insertActionLog(t, db, "al-00000002", "sess1", "create", "issues", "i2",
		`{"title":"Two"}`, `{}`, 0, "")
	insertActionLog(t, db, "al-00000003", "sess1", "update", "issues", "i1",
		`{"title":"Three"}`, `{"title":"One"}`, 0, "")

	// Get rowids for first two rows
	var rowid1, rowid2 int64
	db.QueryRow("SELECT rowid FROM action_log WHERE id = ?", "al-00000001").Scan(&rowid1)
	db.QueryRow("SELECT rowid FROM action_log WHERE id = ?", "al-00000002").Scan(&rowid2)

	acks := []Ack{
		{ClientActionID: rowid1, ServerSeq: 100},
		{ClientActionID: rowid2, ServerSeq: 101},
	}

	tx, _ := db.Begin()
	if err := MarkEventsSynced(tx, acks); err != nil {
		t.Fatalf("MarkEventsSynced: %v", err)
	}
	tx.Commit()

	// Verify synced rows
	var syncedAt sql.NullString
	var serverSeq sql.NullInt64

	db.QueryRow("SELECT synced_at, server_seq FROM action_log WHERE id = ?", "al-00000001").Scan(&syncedAt, &serverSeq)
	if !syncedAt.Valid {
		t.Error("al-00000001: synced_at should be set")
	}
	if !serverSeq.Valid || serverSeq.Int64 != 100 {
		t.Errorf("al-00000001: server_seq got %v, want 100", serverSeq)
	}

	db.QueryRow("SELECT synced_at, server_seq FROM action_log WHERE id = ?", "al-00000002").Scan(&syncedAt, &serverSeq)
	if !syncedAt.Valid {
		t.Error("al-00000002: synced_at should be set")
	}
	if !serverSeq.Valid || serverSeq.Int64 != 101 {
		t.Errorf("al-00000002: server_seq got %v, want 101", serverSeq)
	}

	// Verify unsynced row
	db.QueryRow("SELECT synced_at, server_seq FROM action_log WHERE id = ?", "al-00000003").Scan(&syncedAt, &serverSeq)
	if syncedAt.Valid {
		t.Error("al-00000003: synced_at should NOT be set")
	}
	if serverSeq.Valid {
		t.Error("al-00000003: server_seq should NOT be set")
	}

	// Verify GetPendingEvents now only returns the unsynced one
	tx, _ = db.Begin()
	defer tx.Rollback()
	events, err := GetPendingEvents(tx, "d1", "s1")
	if err != nil {
		t.Fatalf("GetPendingEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("pending events: got %d, want 1", len(events))
	}
}
