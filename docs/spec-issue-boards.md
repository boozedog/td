# Boards Feature: Custom Issue Ordering

## Overview

Add "boards" - named views into issues with per-board custom ordering, separate from priority-based sorting. Enables workflows like sprint backlogs, kanban boards, or implementation order queues.

## Database Schema (Migration v9)

```sql
-- Boards table
CREATE TABLE IF NOT EXISTS boards (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    last_viewed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Board-Issue membership with ordering
CREATE TABLE IF NOT EXISTS board_issues (
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (board_id, issue_id),
    FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_board_issues_position ON board_issues(board_id, position);
```

**Notes:**
- Board IDs use format: `bd-XXXXXXXX` (8 hex chars), enabling short ID input like issues
- `updated_at` must be set explicitly in Go code (SQLite has no auto-update trigger)
- FK cascade on `issue_id`: when issue is hard-deleted (purged), board membership is removed

## Issue Lifecycle in Boards

- **Closing** an issue: Issue remains in board at its position (closed issues visible with status filter)
- **Soft-deleting** an issue (`deleted_at` set): Issue is removed from all boards
- **Hard-deleting** (purge): FK cascade removes board membership automatically

## Ordering Strategy: Contiguous Integer Reindexing

Simple integers (1, 2, 3...) with full reindex on insert/move. Given typical issue counts (<1000), this is sub-millisecond and keeps implementation simple. Positions are **1-indexed**.

## Board Name Validation

- Required, non-empty
- Max 50 characters
- Case-insensitive uniqueness (stored as-is, compared lowercase)
- Allowed: alphanumeric, hyphens, underscores, spaces

## CLI Commands

```
td board                              # Show help (list subcommands)
td board list                         # List all boards
td board list --json                  # List boards as JSON
td board create <name>                # Create board
td board delete <name>                # Delete board
td board show <name>                  # Show board with ordered issues (all statuses)
td board show <name> --status open,in_progress  # Filter by status
td board show <name> --json           # Output as JSON
td board add <name> <id>...           # Add issue(s) to board (at end)
td board add <name> <id> --at <pos>   # Add at specific position (1-indexed)
td board remove <name> <id>           # Remove issue from board
td board move <name> <id> <position>  # Move to absolute position (1-indexed)
td board reorder <name> <id> up|down|top|bottom  # Quick reorder
```

**Position semantics**: `--at 1` inserts at start. Position > count appends at end.

**`reorder` is shorthand**: Internally computes target position and calls move logic.

## TUI Integration

**Approach**: Board mode toggle on TaskList panel

### Keybindings

| Key | Context | Action |
|-----|---------|--------|
| `B` | Main | Open board picker modal |
| `j/k` | Board view | Navigate cursor (normal) |
| `J/K` | Board view | Move selected issue up/down in board (Shift+j/k) |
| `F` | Board view | Open status filter modal |
| `Esc` | Board view | Exit board view, return to default task list |

**Implementation note**: Keymap must detect shift modifier to differentiate `J/K` (reorder) from `j/k` (navigate).

### Board Picker Modal

| Key | Action |
|-----|--------|
| `j/k` | Navigate board list |
| `Enter` | Select board, enter board view |
| `n` | Create new board (opens name input) |
| `Esc` | Cancel, close picker |

### Status Filter Modal

| Key | Action |
|-----|--------|
| `j/k` | Navigate status list |
| `Space` | Toggle status visibility |
| `Enter` | Apply filter, close modal |
| `Esc` | Cancel, close modal |

### Panel Display

- Panel title shows: `BOARD: sprint-1 (5)` when in board mode
- Issues displayed in board position order with position numbers

### Last Viewed Board Persistence

- On board selection, update `boards.last_viewed_at = NOW()`
- On `td monitor` launch, query `GetLastViewedBoard()` - returns board with most recent `last_viewed_at`
- If a last-viewed board exists, auto-enter board mode with that board
- User can exit to default task list with `Esc`

### New State in Model

```go
BoardMode          bool
ActiveBoard        *Board
BoardIssues        []BoardIssue
BoardPickerOpen    bool
BoardPickerCursor  int
AllBoards          []Board           // populated when picker opens
BoardStatusFilter  map[Status]bool   // statuses set to false are hidden; empty map = show all
StatusFilterOpen   bool
StatusFilterCursor int
```

## Models (internal/models/models.go)

```go
// Board types
type Board struct {
    ID           string     `json:"id"`
    Name         string     `json:"name"`
    Description  string     `json:"description,omitempty"`
    LastViewedAt *time.Time `json:"last_viewed_at,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

type BoardIssue struct {
    BoardID  string    `json:"board_id"`
    IssueID  string    `json:"issue_id"`
    Position int       `json:"position"`
    AddedAt  time.Time `json:"added_at"`
}

// ActionType constants for undo support
const (
    ActionBoardCreate      ActionType = "board_create"
    ActionBoardDelete      ActionType = "board_delete"
    ActionBoardAddIssue    ActionType = "board_add_issue"
    ActionBoardRemoveIssue ActionType = "board_remove_issue"
    ActionBoardMoveIssue   ActionType = "board_move_issue"
)
```

## Files to Modify

| File | Changes |
|------|---------|
| `internal/db/schema.go` | Add migration v9: boards + board_issues tables |
| `internal/models/models.go` | Add Board, BoardIssue types + ActionType constants |
| `internal/db/db.go` | CRUD: CreateBoard, GetBoard, GetBoardByName, ListBoards, DeleteBoard, AddIssueToBoard, RemoveIssueFromBoard, MoveIssueInBoard, GetBoardIssues, GetLastViewedBoard, UpdateBoardLastViewed, RemoveIssueFromAllBoards |
| `cmd/board.go` | New file: board commands with subcommands |
| `pkg/monitor/types.go` | Board types, BoardIssuesMsg |
| `pkg/monitor/model.go` | Board state fields, Init() last-viewed restore |
| `pkg/monitor/data.go` | fetchBoardIssues(), fetchBoards() |
| `pkg/monitor/view.go` | renderBoardPanel(), renderBoardPicker(), renderStatusFilter() |
| `pkg/monitor/keymap/bindings.go` | ContextBoardView, CmdMoveIssueUp/Down, CmdOpenBoardPicker, CmdOpenStatusFilter, CmdExitBoardMode |
| `pkg/monitor/commands.go` | Command handlers for board operations |

## Implementation Order

1. **Database layer** - schema.go migration, db.go CRUD operations (including RemoveIssueFromAllBoards for soft-delete hook)
2. **Models** - Board/BoardIssue types, ActionType constants
3. **Soft-delete integration** - Call RemoveIssueFromAllBoards in DeleteIssue()
4. **CLI commands** - cmd/board.go with all subcommands
5. **TUI board picker** - modal to select/create boards
6. **TUI board view** - render board issues in order, status filter
7. **TUI reordering** - J/K to move issues, persist to DB
8. **Last-viewed persistence** - UpdateBoardLastViewed on selection, restore on Init()

## Verification

1. `go build -o td . && go test ./...` - Build and run tests
2. `td board create sprint-1` - Create a board
3. `td board list` - Verify board appears
4. `td board add sprint-1 <id1> <id2> <id3>` - Add issues
5. `td board show sprint-1` - Verify order
6. `td board show sprint-1 --json` - Verify JSON output
7. `td board reorder sprint-1 <id2> top` - Reorder via CLI
8. `td board show sprint-1` - Verify new order
9. `td delete <id2>` - Soft-delete an issue
10. `td board show sprint-1` - Verify deleted issue removed from board
11. `td monitor` → `B` → select board → `J/K` to reorder in TUI
12. `td board show sprint-1` - Verify TUI changes persisted
13. `F` to open status filter → toggle statuses → verify filtering
14. Exit and re-launch `td monitor` - Verify last-viewed board is auto-restored
15. `Esc` to exit board view - Verify returns to default task list
