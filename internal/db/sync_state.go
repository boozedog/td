package db

import (
	"database/sql"
	"time"
)

// SyncState holds the sync configuration for a linked project.
type SyncState struct {
	ProjectID           string
	LastPushedActionID  int64
	LastPulledServerSeq int64
	LastSyncAt          *time.Time
	SyncDisabled        bool
}

// Conn returns the underlying *sql.DB connection for use in transactions
// (e.g., by the sync library which needs raw DB access).
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// GetSyncState returns the current sync state, or nil if the project is not linked.
func (db *DB) GetSyncState() (*SyncState, error) {
	var s SyncState
	var lastSync sql.NullTime
	var disabled int

	err := db.conn.QueryRow(`
		SELECT project_id, last_pushed_action_id, last_pulled_server_seq, last_sync_at, sync_disabled
		FROM sync_state LIMIT 1
	`).Scan(&s.ProjectID, &s.LastPushedActionID, &s.LastPulledServerSeq, &lastSync, &disabled)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastSync.Valid {
		s.LastSyncAt = &lastSync.Time
	}
	s.SyncDisabled = disabled != 0
	return &s, nil
}

// SetSyncState creates or replaces the sync state for a project (used for link).
func (db *DB) SetSyncState(projectID string) error {
	return db.withWriteLock(func() error {
		_, err := db.conn.Exec(`
			INSERT OR REPLACE INTO sync_state (project_id, last_pushed_action_id, last_pulled_server_seq, sync_disabled)
			VALUES (?, 0, 0, 0)
		`, projectID)
		return err
	})
}

// UpdateSyncPushed updates the last pushed action ID and sync time.
func (db *DB) UpdateSyncPushed(lastActionID int64) error {
	return db.withWriteLock(func() error {
		_, err := db.conn.Exec(`
			UPDATE sync_state SET last_pushed_action_id = ?, last_sync_at = CURRENT_TIMESTAMP
		`, lastActionID)
		return err
	})
}

// UpdateSyncPulled updates the last pulled server sequence and sync time.
func (db *DB) UpdateSyncPulled(lastServerSeq int64) error {
	return db.withWriteLock(func() error {
		_, err := db.conn.Exec(`
			UPDATE sync_state SET last_pulled_server_seq = ?, last_sync_at = CURRENT_TIMESTAMP
		`, lastServerSeq)
		return err
	})
}

// ClearSyncState removes the sync state (used for unlink).
func (db *DB) ClearSyncState() error {
	return db.withWriteLock(func() error {
		_, err := db.conn.Exec(`DELETE FROM sync_state`)
		return err
	})
}

// CountPendingEvents returns the number of unsynced action_log entries.
func (db *DB) CountPendingEvents() (int64, error) {
	var count int64
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM action_log WHERE synced_at IS NULL AND undone = 0`).Scan(&count)
	return count, err
}
