package syncharness

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	tdsync "github.com/marcus/td/internal/sync"
)

// clientSchema is the minimal schema needed for sync testing.
const clientSchema = `
CREATE TABLE issues (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open',
    type TEXT NOT NULL DEFAULT 'task',
    priority TEXT NOT NULL DEFAULT 'P2',
    points INTEGER DEFAULT 0,
    labels TEXT DEFAULT '',
    parent_id TEXT DEFAULT '',
    acceptance TEXT DEFAULT '',
    implementer_session TEXT DEFAULT '',
    reviewer_session TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at DATETIME,
    deleted_at DATETIME,
    minor INTEGER DEFAULT 0,
    created_branch TEXT DEFAULT ''
);

CREATE TABLE logs (
    id TEXT PRIMARY KEY,
    issue_id TEXT DEFAULT '',
    session_id TEXT NOT NULL,
    work_session_id TEXT DEFAULT '',
    message TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'progress',
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE handoffs (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    done TEXT DEFAULT '[]',
    remaining TEXT DEFAULT '[]',
    decisions TEXT DEFAULT '[]',
    uncertain TEXT DEFAULT '[]',
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    issue_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE boards (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    last_viewed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    query TEXT NOT NULL DEFAULT '',
    is_builtin INTEGER NOT NULL DEFAULT 0,
    view_mode TEXT NOT NULL DEFAULT 'swimlanes'
);

CREATE TABLE work_sessions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    session_id TEXT NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME,
    start_sha TEXT DEFAULT '',
    end_sha TEXT DEFAULT ''
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

CREATE TABLE sync_state (
    project_id TEXT PRIMARY KEY,
    last_pushed_action_id INTEGER DEFAULT 0,
    last_pulled_server_seq INTEGER DEFAULT 0,
    last_sync_at DATETIME,
    sync_disabled INTEGER DEFAULT 0
);
`

// entityTables lists the tables that hold user data (not action_log or sync_state).
var entityTables = []string{"issues", "logs", "handoffs", "comments", "boards", "work_sessions"}

// validEntities is the set of entity types accepted by the validator.
var validEntities = map[string]bool{
	"issues":        true,
	"logs":          true,
	"handoffs":      true,
	"comments":      true,
	"boards":        true,
	"work_sessions": true,
}

// SimulatedClient represents a single sync client with its own database.
type SimulatedClient struct {
	DeviceID         string
	SessionID        string
	DB               *sql.DB
	LastPushedAction int64
	LastPulledSeq    int64
}

// Harness orchestrates multi-client sync testing.
type Harness struct {
	t          *testing.T
	ProjectDBs map[string]*sql.DB
	Clients    map[string]*SimulatedClient
	clientKeys []string
	Validator  tdsync.EntityValidator
	actionSeq  atomic.Int64
}

// NewHarness creates a test harness with numClients and one server DB for projectID.
func NewHarness(t *testing.T, numClients int, projectID string) *Harness {
	t.Helper()

	h := &Harness{
		t:          t,
		ProjectDBs: make(map[string]*sql.DB),
		Clients:    make(map[string]*SimulatedClient),
		Validator:  func(entityType string) bool { return validEntities[entityType] },
	}

	// Create server DB
	serverDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open server db: %v", err)
	}
	if err := tdsync.InitServerEventLog(serverDB); err != nil {
		t.Fatalf("init server event log: %v", err)
	}
	h.ProjectDBs[projectID] = serverDB
	t.Cleanup(func() { serverDB.Close() })

	// Create clients
	for i := 0; i < numClients; i++ {
		letter := string(rune('A' + i))
		clientID := "client-" + letter
		deviceID := fmt.Sprintf("device-%s-0000-0000-0000-%012d", letter, i+1)
		sessionID := fmt.Sprintf("session-%s-%04d", letter, i+1)

		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("open client %s db: %v", clientID, err)
		}
		if _, err := db.Exec(clientSchema); err != nil {
			t.Fatalf("create schema client %s: %v", clientID, err)
		}
		t.Cleanup(func() { db.Close() })

		h.Clients[clientID] = &SimulatedClient{
			DeviceID:  deviceID,
			SessionID: sessionID,
			DB:        db,
		}
		h.clientKeys = append(h.clientKeys, clientID)
	}

	return h
}

// Mutate performs a local mutation on a client's database and records it in action_log.
func (h *Harness) Mutate(clientID, actionType, entityType, entityID string, data map[string]any) error {
	c, ok := h.Clients[clientID]
	if !ok {
		return fmt.Errorf("unknown client: %s", clientID)
	}

	tx, err := c.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Read previous data
	prevData := readEntity(tx, entityType, entityID)

	switch actionType {
	case "create", "update":
		if err := upsertLocal(tx, entityType, entityID, data); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
	case "delete":
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", entityType), entityID); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
	default:
		return fmt.Errorf("unknown action type: %s", actionType)
	}

	// Build JSON strings
	prevJSON, _ := json.Marshal(prevData)
	var newJSON []byte
	if data != nil {
		newJSON, _ = json.Marshal(data)
	} else {
		newJSON = []byte("{}")
	}

	seq := h.actionSeq.Add(1)
	alID := fmt.Sprintf("al-%08d", seq)

	_, err = tx.Exec(
		`INSERT INTO action_log (id, session_id, action_type, entity_type, entity_id, new_data, previous_data, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		alID, c.SessionID, actionType, entityType, entityID, string(newJSON), string(prevJSON),
	)
	if err != nil {
		return fmt.Errorf("insert action_log: %w", err)
	}

	return tx.Commit()
}

// readEntity reads all columns for a given entity, returning a map.
func readEntity(tx *sql.Tx, entityType, entityID string) map[string]any {
	row, err := tx.Query(fmt.Sprintf("SELECT * FROM %s WHERE id = ?", entityType), entityID)
	if err != nil {
		return nil
	}
	defer row.Close()

	if !row.Next() {
		return nil
	}

	cols, err := row.Columns()
	if err != nil {
		return nil
	}

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		return nil
	}

	result := make(map[string]any, len(cols))
	for i, col := range cols {
		result[col] = vals[i]
	}
	return result
}

// upsertLocal inserts or replaces a row in the entity table.
func upsertLocal(tx *sql.Tx, entityType, entityID string, data map[string]any) error {
	fields := make(map[string]any, len(data)+1)
	for k, v := range data {
		fields[k] = v
	}
	fields["id"] = entityID

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	placeholders := make([]string, len(keys))
	vals := make([]any, len(keys))
	for i, k := range keys {
		placeholders[i] = "?"
		vals[i] = fields[k]
	}

	query := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		entityType, strings.Join(keys, ", "), strings.Join(placeholders, ", "))

	_, err := tx.Exec(query, vals...)
	return err
}

// Push sends pending events from a client to the server.
func (h *Harness) Push(clientID, projectID string) (tdsync.PushResult, error) {
	c, ok := h.Clients[clientID]
	if !ok {
		return tdsync.PushResult{}, fmt.Errorf("unknown client: %s", clientID)
	}
	serverDB, ok := h.ProjectDBs[projectID]
	if !ok {
		return tdsync.PushResult{}, fmt.Errorf("unknown project: %s", projectID)
	}

	// Read pending events from client
	clientTx, err := c.DB.Begin()
	if err != nil {
		return tdsync.PushResult{}, fmt.Errorf("begin client tx: %w", err)
	}

	events, err := tdsync.GetPendingEvents(clientTx, c.DeviceID, c.SessionID)
	if err != nil {
		clientTx.Rollback()
		return tdsync.PushResult{}, fmt.Errorf("get pending: %w", err)
	}

	if len(events) == 0 {
		clientTx.Rollback()
		return tdsync.PushResult{}, nil
	}

	// Insert into server
	serverTx, err := serverDB.Begin()
	if err != nil {
		clientTx.Rollback()
		return tdsync.PushResult{}, fmt.Errorf("begin server tx: %w", err)
	}

	result, err := tdsync.InsertServerEvents(serverTx, events)
	if err != nil {
		serverTx.Rollback()
		clientTx.Rollback()
		return tdsync.PushResult{}, fmt.Errorf("insert server events: %w", err)
	}

	if err := serverTx.Commit(); err != nil {
		clientTx.Rollback()
		return tdsync.PushResult{}, fmt.Errorf("commit server tx: %w", err)
	}

	// Mark synced on client
	if err := tdsync.MarkEventsSynced(clientTx, result.Acks); err != nil {
		clientTx.Rollback()
		return tdsync.PushResult{}, fmt.Errorf("mark synced: %w", err)
	}

	if err := clientTx.Commit(); err != nil {
		return tdsync.PushResult{}, fmt.Errorf("commit client tx: %w", err)
	}

	// Update last pushed action
	if len(result.Acks) > 0 {
		c.LastPushedAction = result.Acks[len(result.Acks)-1].ClientActionID
	}

	return result, nil
}

// Pull fetches new events from the server and applies them to the client.
func (h *Harness) Pull(clientID, projectID string) (tdsync.PullResult, error) {
	c, ok := h.Clients[clientID]
	if !ok {
		return tdsync.PullResult{}, fmt.Errorf("unknown client: %s", clientID)
	}
	serverDB, ok := h.ProjectDBs[projectID]
	if !ok {
		return tdsync.PullResult{}, fmt.Errorf("unknown project: %s", projectID)
	}

	// Get events from server
	serverTx, err := serverDB.Begin()
	if err != nil {
		return tdsync.PullResult{}, fmt.Errorf("begin server tx: %w", err)
	}

	pullResult, err := tdsync.GetEventsSince(serverTx, c.LastPulledSeq, 10000, c.DeviceID)
	if err != nil {
		serverTx.Rollback()
		return tdsync.PullResult{}, fmt.Errorf("get events since: %w", err)
	}

	if err := serverTx.Commit(); err != nil {
		return tdsync.PullResult{}, fmt.Errorf("commit server tx: %w", err)
	}

	if len(pullResult.Events) == 0 {
		return pullResult, nil
	}

	// Apply to client
	clientTx, err := c.DB.Begin()
	if err != nil {
		return tdsync.PullResult{}, fmt.Errorf("begin client tx: %w", err)
	}

	applyResult, err := tdsync.ApplyRemoteEvents(clientTx, pullResult.Events, c.DeviceID, h.Validator)
	if err != nil {
		clientTx.Rollback()
		return tdsync.PullResult{}, fmt.Errorf("apply remote events: %w", err)
	}

	if err := clientTx.Commit(); err != nil {
		return tdsync.PullResult{}, fmt.Errorf("commit client tx: %w", err)
	}

	// Update last pulled seq
	if applyResult.LastAppliedSeq > c.LastPulledSeq {
		c.LastPulledSeq = applyResult.LastAppliedSeq
	}
	if pullResult.LastServerSeq > c.LastPulledSeq {
		c.LastPulledSeq = pullResult.LastServerSeq
	}

	return pullResult, nil
}

// Sync pushes then pulls for a client.
func (h *Harness) Sync(clientID, projectID string) error {
	if _, err := h.Push(clientID, projectID); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if _, err := h.Pull(clientID, projectID); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return nil
}

// AssertConverged verifies all clients have identical entity data.
func (h *Harness) AssertConverged(projectID string) {
	h.t.Helper()

	if len(h.clientKeys) < 2 {
		return
	}

	for _, table := range entityTables {
		var refRows string
		var refClient string
		for i, clientID := range h.clientKeys {
			rows := dumpTable(h.Clients[clientID].DB, table)
			if i == 0 {
				refRows = rows
				refClient = clientID
				continue
			}
			if rows != refRows {
				h.t.Fatalf("DIVERGENCE in table %q between %s and %s:\n--- %s ---\n%s\n--- %s ---\n%s",
					table, refClient, clientID, refClient, refRows, clientID, rows)
			}
		}
	}
}

// Diff returns a human-readable diff of entity tables between two clients.
func (h *Harness) Diff(clientA, clientB string) string {
	cA, okA := h.Clients[clientA]
	cB, okB := h.Clients[clientB]
	if !okA || !okB {
		return fmt.Sprintf("unknown client(s): %s, %s", clientA, clientB)
	}

	var sb strings.Builder
	for _, table := range entityTables {
		rowsA := dumpTable(cA.DB, table)
		rowsB := dumpTable(cB.DB, table)
		if rowsA != rowsB {
			sb.WriteString(fmt.Sprintf("=== %s ===\n", table))
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n", clientA, rowsA))
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n", clientB, rowsB))
		}
	}
	if sb.Len() == 0 {
		return "(identical)"
	}
	return sb.String()
}

// timestampCols are excluded from convergence checks because INSERT OR REPLACE
// sets DEFAULT CURRENT_TIMESTAMP independently on each client.
var timestampCols = map[string]bool{
	"created_at": true, "updated_at": true, "closed_at": true,
	"deleted_at": true, "timestamp": true, "started_at": true,
	"ended_at": true, "last_viewed_at": true, "linked_at": true,
	"tagged_at": true, "added_at": true,
}

// dumpTable returns a deterministic string representation of all rows in a table.
// Timestamp columns are excluded from the dump to avoid false divergence.
func dumpTable(db *sql.DB, table string) string {
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s ORDER BY id", table))
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}

	var sb strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			sb.WriteString(fmt.Sprintf("SCAN ERROR: %v\n", err))
			continue
		}

		var parts []string
		for i, col := range cols {
			if timestampCols[col] {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%v", col, vals[i]))
		}
		sb.WriteString(strings.Join(parts, " | "))
		sb.WriteString("\n")
	}
	return sb.String()
}

// QueryEntity reads a single entity from a client's DB, returning nil if not found.
func (h *Harness) QueryEntity(clientID, entityType, entityID string) map[string]any {
	h.t.Helper()
	c, ok := h.Clients[clientID]
	if !ok {
		h.t.Fatalf("unknown client: %s", clientID)
	}

	tx, err := c.DB.Begin()
	if err != nil {
		h.t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	return readEntity(tx, entityType, entityID)
}

// CountEntities returns the number of rows in an entity table for a client.
func (h *Harness) CountEntities(clientID, entityType string) int {
	h.t.Helper()
	c, ok := h.Clients[clientID]
	if !ok {
		h.t.Fatalf("unknown client: %s", clientID)
	}

	var count int
	err := c.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", entityType)).Scan(&count)
	if err != nil {
		h.t.Fatalf("count %s: %v", entityType, err)
	}
	return count
}

// PullAll fetches all new events from the server (including own device) and applies them.
// This ensures convergence by replaying events in server-seq order regardless of origin.
func (h *Harness) PullAll(clientID, projectID string) (tdsync.PullResult, error) {
	c, ok := h.Clients[clientID]
	if !ok {
		return tdsync.PullResult{}, fmt.Errorf("unknown client: %s", clientID)
	}
	serverDB, ok := h.ProjectDBs[projectID]
	if !ok {
		return tdsync.PullResult{}, fmt.Errorf("unknown project: %s", projectID)
	}

	serverTx, err := serverDB.Begin()
	if err != nil {
		return tdsync.PullResult{}, fmt.Errorf("begin server tx: %w", err)
	}

	// Empty excludeDevice = get all events including own
	pullResult, err := tdsync.GetEventsSince(serverTx, c.LastPulledSeq, 10000, "")
	if err != nil {
		serverTx.Rollback()
		return tdsync.PullResult{}, fmt.Errorf("get events since: %w", err)
	}

	if err := serverTx.Commit(); err != nil {
		return tdsync.PullResult{}, fmt.Errorf("commit server tx: %w", err)
	}

	if len(pullResult.Events) == 0 {
		return pullResult, nil
	}

	clientTx, err := c.DB.Begin()
	if err != nil {
		return tdsync.PullResult{}, fmt.Errorf("begin client tx: %w", err)
	}

	applyResult, err := tdsync.ApplyRemoteEvents(clientTx, pullResult.Events, c.DeviceID, h.Validator)
	if err != nil {
		clientTx.Rollback()
		return tdsync.PullResult{}, fmt.Errorf("apply remote events: %w", err)
	}

	if err := clientTx.Commit(); err != nil {
		return tdsync.PullResult{}, fmt.Errorf("commit client tx: %w", err)
	}

	if applyResult.LastAppliedSeq > c.LastPulledSeq {
		c.LastPulledSeq = applyResult.LastAppliedSeq
	}
	if pullResult.LastServerSeq > c.LastPulledSeq {
		c.LastPulledSeq = pullResult.LastServerSeq
	}

	return pullResult, nil
}
