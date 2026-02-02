package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/marcus/td/internal/models"
)

// marshalBoard returns a JSON representation of a board for action_log storage.
func marshalBoard(board *models.Board) string {
	data, _ := json.Marshal(board)
	return string(data)
}

// scanBoardRow reads a board row from the DB within a withWriteLock closure.
func (db *DB) scanBoardRow(id string) (*models.Board, error) {
	board, err := db.GetBoard(id)
	if err != nil {
		return nil, err
	}
	return board, nil
}

// CreateBoardLogged creates a board and logs the action atomically within a single withWriteLock call.
func (db *DB) CreateBoardLogged(name, queryStr, sessionID string) (*models.Board, error) {
	var board *models.Board
	err := db.withWriteLock(func() error {
		// Validate query syntax if not empty
		if queryStr != "" {
			if err := parseAndValidateQuery(queryStr); err != nil {
				return fmt.Errorf("invalid query: %w", err)
			}
		}

		id, err := generateBoardID()
		if err != nil {
			return err
		}

		now := time.Now()
		board = &models.Board{
			ID:        id,
			Name:      name,
			Query:     queryStr,
			IsBuiltin: false,
			ViewMode:  "swimlanes",
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err = db.conn.Exec(`
			INSERT INTO boards (id, name, query, is_builtin, view_mode, created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, ?)
		`, board.ID, board.Name, board.Query, board.ViewMode, board.CreatedAt, board.UpdatedAt)
		if err != nil {
			return err
		}

		// Log the action
		actionID, err := generateActionID()
		if err != nil {
			return fmt.Errorf("generate action ID: %w", err)
		}
		newData := marshalBoard(board)
		_, err = db.conn.Exec(`INSERT INTO action_log (id, session_id, action_type, entity_type, entity_id, previous_data, new_data, timestamp, undone) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			actionID, sessionID, string(models.ActionBoardCreate), "board", board.ID, "", newData, now)
		if err != nil {
			return fmt.Errorf("log action: %w", err)
		}

		return nil
	})
	return board, err
}

// UpdateBoardLogged updates a board and logs the action atomically within a single withWriteLock call.
func (db *DB) UpdateBoardLogged(board *models.Board, sessionID string) error {
	return db.withWriteLock(func() error {
		// Read current state for PreviousData
		prev, err := db.scanBoardRow(board.ID)
		if err != nil {
			return err
		}
		previousData := marshalBoard(prev)

		// Check if builtin
		if prev.IsBuiltin {
			return fmt.Errorf("cannot modify builtin board")
		}

		// Validate query if provided
		if board.Query != "" {
			if err := parseAndValidateQuery(board.Query); err != nil {
				return fmt.Errorf("invalid query: %w", err)
			}
		}

		board.UpdatedAt = time.Now()
		_, err = db.conn.Exec(`
			UPDATE boards SET name = ?, query = ?, updated_at = ?
			WHERE id = ?
		`, board.Name, board.Query, board.UpdatedAt, board.ID)
		if err != nil {
			return err
		}

		// Log the action
		actionID, err := generateActionID()
		if err != nil {
			return fmt.Errorf("generate action ID: %w", err)
		}
		newData := marshalBoard(board)
		_, err = db.conn.Exec(`INSERT INTO action_log (id, session_id, action_type, entity_type, entity_id, previous_data, new_data, timestamp, undone) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			actionID, sessionID, string(models.ActionBoardUpdate), "board", board.ID, previousData, newData, board.UpdatedAt)
		if err != nil {
			return fmt.Errorf("log action: %w", err)
		}

		return nil
	})
}

// DeleteBoardLogged deletes a board and logs the action atomically within a single withWriteLock call.
func (db *DB) DeleteBoardLogged(boardID, sessionID string) error {
	return db.withWriteLock(func() error {
		// Read current state for PreviousData
		prev, err := db.scanBoardRow(boardID)
		if err != nil {
			return err
		}
		if prev.IsBuiltin {
			return fmt.Errorf("cannot delete builtin board")
		}
		previousData := marshalBoard(prev)

		// Soft-delete positions first
		now := time.Now()
		_, err = db.conn.Exec(`UPDATE board_issue_positions SET deleted_at = ? WHERE board_id = ? AND deleted_at IS NULL`, now.UTC(), boardID)
		if err != nil {
			return err
		}

		// Delete board
		_, err = db.conn.Exec(`DELETE FROM boards WHERE id = ?`, boardID)
		if err != nil {
			return err
		}

		// Log the action
		actionID, err := generateActionID()
		if err != nil {
			return fmt.Errorf("generate action ID: %w", err)
		}
		_, err = db.conn.Exec(`INSERT INTO action_log (id, session_id, action_type, entity_type, entity_id, previous_data, new_data, timestamp, undone) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			actionID, sessionID, string(models.ActionBoardDelete), "board", boardID, previousData, "", now)
		if err != nil {
			return fmt.Errorf("log action: %w", err)
		}

		return nil
	})
}
