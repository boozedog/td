# CLAUDE.md

## MANDATORY: Use `td` for Task Management

Run `td usage --new-session` at conversation start (or after /clear). This tells you what to work on next.

Sessions are automatic (based on your terminal/agent context). Optional:
- `td session "name"` to label the current session
- `td session --new` to force a new session in the same context

Use `td usage -q` after first read.

## Build

```bash
go build -o td .           # Build
go test ./...              # Test all
```

## Architecture

- `cmd/` - Cobra commands
- `internal/db/` - SQLite (schema.go)
- `internal/models/` - Issue, Log, Handoff, WorkSession
- `internal/session/` - Session ID (.todos/session)

Issue lifecycle: open → in_progress → in_review → closed (or blocked)

## Undo Support

Log actions via `database.LogAction()`. See `cmd/undo.go` for implementation.
