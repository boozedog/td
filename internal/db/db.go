package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/marcus/td/internal/workdir"
	_ "modernc.org/sqlite"
)

// QueryValidator is set by main to validate TDQ queries without import cycle.
// Returns nil if valid, error describing parse failure otherwise.
var QueryValidator func(queryStr string) error

const (
	dbFile = ".todos/issues.db"
)

// DB wraps the database connection
type DB struct {
	conn    *sql.DB
	baseDir string
}

// ResolveBaseDir checks for a .td-root file in the given directory.
// If found, it returns the path contained in that file (pointing to the main
// worktree's root). Otherwise, returns the original baseDir unchanged.
// This enables git worktrees to share a single td database with the main repo.
func ResolveBaseDir(baseDir string) string {
	return workdir.ResolveBaseDir(baseDir)
}

// Open opens the database and runs any pending migrations
func Open(baseDir string) (*DB, error) {
	// Check for worktree redirection via .td-root
	baseDir = ResolveBaseDir(baseDir)
	dbPath := filepath.Join(baseDir, dbFile)

	// Check if db exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not found: run 'td init' first")
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads while writes are serialized
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Set busy timeout as fallback protection (500ms, matches lock timeout)
	if _, err := conn.Exec("PRAGMA busy_timeout=500"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Slightly faster writes, still safe with WAL
	conn.Exec("PRAGMA synchronous=NORMAL")

	db := &DB{conn: conn, baseDir: baseDir}

	// Run any pending migrations
	if _, err := db.RunMigrations(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// Initialize creates the database and runs migrations
func Initialize(baseDir string) (*DB, error) {
	// Check for worktree redirection via .td-root
	baseDir = ResolveBaseDir(baseDir)
	dbPath := filepath.Join(baseDir, dbFile)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads while writes are serialized
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Set busy timeout as fallback protection (500ms, matches lock timeout)
	if _, err := conn.Exec("PRAGMA busy_timeout=500"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Slightly faster writes, still safe with WAL
	conn.Exec("PRAGMA synchronous=NORMAL")

	// Run schema
	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	db := &DB{conn: conn, baseDir: baseDir}

	// Run migrations
	if _, err := db.RunMigrations(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// Close closes the database
func (db *DB) Close() error {
	return db.conn.Close()
}

// SetMaxOpenConns sets the maximum number of open connections to the database.
// For SQLite with single-writer semantics, this should typically be set to 1
// to prevent connection pool growth in long-running applications.
func (db *DB) SetMaxOpenConns(n int) {
	db.conn.SetMaxOpenConns(n)
}

// BaseDir returns the base directory for the database
func (db *DB) BaseDir() string {
	return db.baseDir
}

// withWriteLock executes fn while holding an exclusive write lock.
// This prevents concurrent writes from multiple processes.
func (db *DB) withWriteLock(fn func() error) error {
	locker := newWriteLocker(db.baseDir)
	if err := locker.acquire(defaultTimeout); err != nil {
		return err
	}
	defer locker.release()
	return fn()
}
