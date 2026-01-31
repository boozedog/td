package sync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validColumnName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ApplyEvent applies a single sync event to the database within the given transaction.
// The validator is called to check that the entity type is allowed before any SQL is executed.
func ApplyEvent(tx *sql.Tx, event Event, validator EntityValidator) error {
	if !validator(event.EntityType) {
		return fmt.Errorf("invalid entity type: %q", event.EntityType)
	}

	if event.EntityID == "" {
		return fmt.Errorf("empty entity ID for %q event", event.ActionType)
	}

	switch event.ActionType {
	case "create", "update":
		return upsertEntity(tx, event.EntityType, event.EntityID, event.Payload)
	case "delete":
		return deleteEntity(tx, event.EntityType, event.EntityID)
	case "soft_delete":
		return softDeleteEntity(tx, event.EntityType, event.EntityID, event.ClientTimestamp)
	default:
		return fmt.Errorf("unknown action type: %q", event.ActionType)
	}
}

// upsertEntity inserts or replaces a row using the JSON payload fields.
func upsertEntity(tx *sql.Tx, entityType, entityID string, newData json.RawMessage) error {
	if newData == nil {
		return fmt.Errorf("upsert %s/%s: nil payload", entityType, entityID)
	}

	var fields map[string]any
	if err := json.Unmarshal(newData, &fields); err != nil {
		return fmt.Errorf("upsert %s/%s: unmarshal payload: %w", entityType, entityID, err)
	}

	if len(fields) == 0 {
		return fmt.Errorf("upsert %s/%s: payload has no fields", entityType, entityID)
	}

	fields["id"] = entityID

	cols, placeholders, vals, err := buildInsert(fields)
	if err != nil {
		return fmt.Errorf("upsert %s/%s: %w", entityType, entityID, err)
	}
	query := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)", entityType, cols, placeholders)

	slog.Debug("upsert", "table", entityType, "id", entityID)
	if _, err := tx.Exec(query, vals...); err != nil {
		return fmt.Errorf("upsert %s/%s: %w", entityType, entityID, err)
	}
	return nil
}

// deleteEntity hard-deletes a row. No-op if the row does not exist.
func deleteEntity(tx *sql.Tx, entityType, entityID string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", entityType)
	if _, err := tx.Exec(query, entityID); err != nil {
		return fmt.Errorf("delete %s/%s: %w", entityType, entityID, err)
	}
	return nil
}

// softDeleteEntity sets deleted_at on a row. No-op if the row does not exist.
func softDeleteEntity(tx *sql.Tx, entityType, entityID string, timestamp time.Time) error {
	query := fmt.Sprintf("UPDATE %s SET deleted_at = ? WHERE id = ?", entityType)
	if _, err := tx.Exec(query, timestamp, entityID); err != nil {
		return fmt.Errorf("soft_delete %s/%s: %w", entityType, entityID, err)
	}
	return nil
}

// buildInsert sorts fields alphabetically and returns column list, placeholders, and values.
// Returns an error if any key is not a valid SQL column name.
func buildInsert(fields map[string]any) (cols string, placeholders string, vals []any, err error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if !validColumnName.MatchString(k) {
			return "", "", nil, fmt.Errorf("invalid column name: %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ph := make([]string, len(keys))
	vals = make([]any, len(keys))
	for i, k := range keys {
		ph[i] = "?"
		vals[i] = fields[k]
	}

	cols = strings.Join(keys, ", ")
	placeholders = strings.Join(ph, ", ")
	return
}
