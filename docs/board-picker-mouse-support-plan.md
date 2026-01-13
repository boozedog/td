# Plan: Add Mouse Support to Board Picker Modal

## Summary

Add mouse support to the board picker modal following patterns consistent with existing TD modals and Sidecar's modal implementations.

## Current State

**Board Picker** (`pkg/monitor/`):
- `view.go:1479-1561`: `renderBoardPicker()`, `wrapBoardPickerModal()`
- `model.go:100-102`: `BoardPickerOpen`, `BoardPickerCursor`, `AllBoards`
- `input.go:773-898`: `handleMouse()` - main mouse router (board picker NOT handled)
- No mouse support currently

**Existing TD Mouse Patterns**:
- `handleConfirmDialogClick()` - calculates modal bounds, tests button positions
- `handleFormDialogClick()` - similar pattern with hover state
- Uses inline bounds calculation, stores hover as int fields on Model

## Implementation Plan

### 1. Add Hover State Field

**File**: `pkg/monitor/model.go`

Add after line 101:
```go
BoardPickerHover int // -1=none, 0+=hovered board index
```

### 2. Create Board Picker Mouse Handler

**File**: `pkg/monitor/input.go`

Add new function `handleBoardPickerClick(x, y int)`:
- Calculate modal dimensions (60% of terminal, capped 40-80 width, 10-30 height)
- Center modal: `modalX = (m.Width - modalWidth) / 2`
- Calculate content bounds accounting for border + padding + header
- Convert click Y to board index
- Single-click selects AND activates board (matches confirmation dialog pattern)
- Click outside modal closes it

Add new function `handleBoardPickerHover(x, y int)`:
- Same bounds calculation
- Update `BoardPickerHover` based on which row is under cursor
- Return -1 if outside content area

### 3. Integrate into Mouse Router

**File**: `pkg/monitor/input.go` in `handleMouse()`

Add after form dialog handling (~line 836):
```go
// Board picker click
if m.BoardPickerOpen && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
    return m.handleBoardPickerClick(msg.X, msg.Y)
}

// Board picker hover
if m.BoardPickerOpen && msg.Action == tea.MouseActionMotion {
    return m.handleBoardPickerHover(msg.X, msg.Y)
}
```

Add `BoardPickerOpen` to the modal-blocks-events condition (~line 850).

### 4. Add Scroll Wheel Support

**File**: `pkg/monitor/input.go` in `handleMouse()`

Add in scroll handling section (~line 812):
```go
if m.BoardPickerOpen {
    // Move cursor up/down based on scroll direction
    delta := -3 // scroll up moves cursor up
    if msg.Button == tea.MouseButtonWheelDown {
        delta = 3
    }
    newCursor := m.BoardPickerCursor + delta
    // Clamp to valid range
    m.BoardPickerCursor = clamp(newCursor, 0, len(m.AllBoards)-1)
    return m, nil
}
```

### 5. Update Rendering for Hover State

**File**: `pkg/monitor/view.go` in `renderBoardPicker()`

Update board row rendering to show hover state:
- Current selection: highlighted (existing)
- Hover (not selected): subtle highlight style
- Normal: default style

### 6. Initialize Hover State

**File**: `pkg/monitor/commands.go` in `openBoardPicker()`

Add: `m.BoardPickerHover = -1`

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/monitor/model.go` | Add `BoardPickerHover` field |
| `pkg/monitor/input.go` | Add handlers, integrate into router |
| `pkg/monitor/view.go` | Update rendering for hover state |
| `pkg/monitor/commands.go` | Initialize hover state |

## Mouse Behavior

| Action | Result |
|--------|--------|
| Click on board row | Select and activate that board |
| Click outside modal | Close picker |
| Hover over board row | Visual highlight |
| Scroll wheel | Move cursor up/down |

## Verification

1. Run `td` and open board picker (`b` key)
2. Hover over boards - verify hover highlighting
3. Click a board - verify it activates
4. Click outside modal - verify it closes
5. Scroll wheel - verify cursor moves
6. Keyboard still works - verify j/k/Enter/Esc
