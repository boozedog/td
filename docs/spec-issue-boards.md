# Boards Feature: Custom Issue Ordering

## Overview

Add "boards" - named views into issues with per-board custom ordering, separate from priority-based sorting. Enables workflows like sprint backlogs, kanban boards, or implementation order queues.

## Database Schema (Migration v9)

```sql
-- Boards table
CREATE TABLE IF NOT EXISTS boards (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
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

CREATE UNIQUE INDEX IF NOT EXISTS idx_board_issues_position ON board_issues(board_id, position);
```

**Notes:**
- Board IDs use format: `bd-XXXXXXXX` (8 hex chars), enabling short ID input like issues
- `updated_at` must be set explicitly in Go code (SQLite has no auto-update trigger); update on rename/delete and membership/order changes
- FK cascade on `issue_id`: when issue is hard-deleted (purged), board membership is removed
- Board name lookups use case-insensitive comparisons (backed by `COLLATE NOCASE`)

## Issue Lifecycle in Boards

- **Closing** an issue: Issue remains in board at its position (closed issues visible with status filter)
- **Soft-deleting** an issue (`deleted_at` set): Board membership is preserved, but deleted issues are excluded from board views
- **Restoring** an issue: Issue reappears in boards at its original position
- **Hard-deleting** (purge): FK cascade removes board membership automatically

## Ordering Strategy: Contiguous Integer Reindexing

Simple integers (1, 2, 3...) with full reindex on insert/move. Given typical issue counts (<1000), this is sub-millisecond and keeps implementation simple. Positions are **1-indexed**.

**Reindexing**: wrap add/move/remove in a transaction. To avoid unique index conflicts, apply a temporary offset (e.g., `position = position + 10000`) before writing final contiguous positions.

## Board Name Validation

- Required, non-empty (trim leading/trailing whitespace)
- Max 50 characters
- Case-insensitive uniqueness (stored as-is, enforced via `COLLATE NOCASE`)
- Allowed: alphanumeric, hyphens, underscores, spaces
- Disallow names matching `^bd-[0-9a-f]{8}$` to keep board references unambiguous

## CLI Commands

```
td board                              # Show help (list subcommands)
td board list                         # List all boards
td board list --json                  # List boards as JSON
td board create <board>               # Create board
td board delete <board>               # Delete board
td board show <board>                 # Show board with ordered issues (all statuses)
td board show <board> --status open,in_progress  # Filter by status
td board show <board> --json          # Output as JSON
td board add <board> <id>...          # Add issue(s) to board (at end)
td board add <board> <id> --at <pos>  # Add at specific position (1-indexed)
td board remove <board> <id>          # Remove issue from board
td board move <board> <id> <position> # Move to absolute position (1-indexed)
td board reorder <board> <id> up|down|top|bottom # Quick reorder
```

**Board references**: `<board>` accepts board name or `bd-XXXXXXXX` ID. If the name contains spaces, quote it.

**List output**: `td board list` includes board IDs for copy/paste.

**JSON output**: `td board show --json` returns `{ "board": Board, "issues": []BoardIssueView }` with `issue` populated and original `position` preserved.

**Position semantics**: `--at 1` inserts at start. Position > count appends at end.

**Filtering**: status filters do not renumber positions; gaps are expected.

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
- Deleted issues are always hidden; status filter applies only to non-deleted issues
- Filters do not renumber positions; display original board positions (gaps allowed)

### Last Viewed Board Persistence

- On board selection, update `boards.last_viewed_at = NOW()`
- On `td monitor` launch, query `GetLastViewedBoard()` - returns board with most recent `last_viewed_at`
- If a last-viewed board exists, auto-enter board mode with that board
- User can exit to default task list with `Esc`

### New State in Model

```go
BoardMode          bool
ActiveBoard        *Board
BoardIssues        []BoardIssueView
BoardPickerOpen    bool
BoardPickerCursor  int
AllBoards          []Board           // populated when picker opens
BoardStatusFilter  map[Status]bool   // statuses set to false are hidden; deleted issues are always hidden
StatusFilterOpen   bool
StatusFilterCursor int
```

## Models (internal/models/models.go)

```go
// Board types
type Board struct {
    ID           string     `json:"id"`
    Name         string     `json:"name"`
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

type BoardIssueView struct {
    BoardID  string    `json:"board_id"`
    Position int       `json:"position"`
    AddedAt  time.Time `json:"added_at"`
    Issue    Issue     `json:"issue"`
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
| `internal/models/models.go` | Add Board, BoardIssue, BoardIssueView types + ActionType constants |
| `internal/db/db.go` | CRUD: CreateBoard, GetBoard, GetBoardByName, ResolveBoardRef, ListBoards, DeleteBoard, AddIssueToBoard, RemoveIssueFromBoard, MoveIssueInBoard, GetBoardIssues (returns BoardIssueView, excludes deleted), GetLastViewedBoard, UpdateBoardLastViewed |
| `cmd/board.go` | New file: board commands with subcommands |
| `pkg/monitor/types.go` | Board types, BoardIssueView, BoardIssuesMsg |
| `pkg/monitor/model.go` | Board state fields, Init() last-viewed restore |
| `pkg/monitor/data.go` | fetchBoardIssues(), fetchBoards() |
| `pkg/monitor/view.go` | renderBoardPanel(), renderBoardPicker(), renderStatusFilter() |
| `pkg/monitor/keymap/bindings.go` | ContextBoardView, CmdMoveIssueUp/Down, CmdOpenBoardPicker, CmdOpenStatusFilter, CmdExitBoardMode |
| `pkg/monitor/commands.go` | Command handlers for board operations |

## Implementation Order

1. **Database layer** - schema.go migration, db.go CRUD operations (including ResolveBoardRef and joined board issue queries)
2. **Models** - Board/BoardIssue/BoardIssueView types, ActionType constants
3. **Soft-delete integration** - Ensure board issue queries filter `deleted_at IS NULL` so deleted issues are hidden
4. **CLI commands** - cmd/board.go with all subcommands
5. **TUI board picker** - modal to select/create boards
6. **TUI board view** - render board issues in order, status filter
7. **TUI reordering** - J/K to move issues, persist to DB
8. **Last-viewed persistence** - UpdateBoardLastViewed on selection, restore on Init()

## Verification

1. `go build -o td . && go test ./...` - Build and run tests
2. `td board create sprint-1` - Create a board
3. `td board list` - Verify board appears
4. `td board show <bd-id-from-list>` - Verify board ID references work
5. `td board add sprint-1 <id1> <id2> <id3>` - Add issues
6. `td board show sprint-1` - Verify order
7. `td board show sprint-1 --json` - Verify JSON output
8. `td board reorder sprint-1 <id2> top` - Reorder via CLI
9. `td board show sprint-1` - Verify new order
10. `td delete <id2>` - Soft-delete an issue
11. `td board show sprint-1` - Verify deleted issue hidden but positions preserved
12. `td restore <id2>` - Restore issue
13. `td board show sprint-1` - Verify restored issue returns to same board position
14. `td monitor` → `B` → select board → `J/K` to reorder in TUI
15. `td board show sprint-1` - Verify TUI changes persisted
16. `F` to open status filter → toggle statuses → verify filtering
17. Exit and re-launch `td monitor` - Verify last-viewed board is auto-restored
18. `Esc` to exit board view - Verify returns to default task list
