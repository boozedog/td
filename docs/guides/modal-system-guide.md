# Modal System Architecture

Guide for developers adding modal-related features to the TD monitor.

## Core Concepts

### Modal Stack

Modals use a stack architecture (`ModalStack []ModalEntry`) allowing nested navigation:

```go
type ModalEntry struct {
    IssueID     string
    SourcePanel Panel      // Only meaningful for base modal (depth 1)
    Scroll      int

    // Async data
    Loading, Error, Issue, Handoff, Logs, BlockedBy, Blocks
    DescRender, AcceptRender  // Pre-rendered markdown

    // Epic-specific
    EpicTasks          []models.Issue
    EpicTasksCursor    int
    TaskSectionFocused bool
}
```

### Helper Methods

```go
m.ModalOpen()       // bool - any modal open?
m.ModalDepth()      // int - stack depth (0 = none)
m.CurrentModal()    // *ModalEntry - top of stack (nil if empty)
m.ModalSourcePanel() // Panel - base modal's source panel
m.ModalBreadcrumb()  // string - "epic: td-001 > task: td-002"
```

## Overlay Implementation

Modals dim the background to:
- Draw user focus to the modal content
- Provide visual separation between modal and underlying content
- Show context from the underlying panels while focusing attention on the modal

### Dimmed Background Overlay

All modals use `OverlayModal()` from `pkg/monitor/overlay.go` to show **dimmed background content** behind the modal:

```go
// In renderView():
if m.FormOpen && m.FormState != nil {
    background := m.renderBaseView()
    form := m.renderFormModal()
    return OverlayModal(background, form, m.Width, m.Height)
}
```

**How `OverlayModal()` works:**
1. Renders the base view (panels + footer) via `renderBaseView()`
2. Strips ANSI codes from background and applies dim gray styling (color 242)
3. Calculates modal position (centered horizontally and vertically)
4. Composites each row: `dimmed-left + modal + dimmed-right`
5. Shows dimmed background on all four sides of the modal

**Visual result:**
```
╔════════════════════════════════════════════════╗
║  [dimmed gray background text]                 ║
║  [gray left]  ┌─Modal─┐  [gray right]          ║
║  [gray left]  │ text  │  [gray right]          ║
║  [gray left]  └───────┘  [gray right]          ║
║  [dimmed gray background text]                 ║
╚════════════════════════════════════════════════╝
```

**Note:** Background colors are not preserved because ANSI SGR 2 (faint) doesn't reliably combine with existing color codes in most terminals. The gray overlay provides consistent dimming.

### Alternative: Solid Black Overlay

Use `lipgloss.Place()` with whitespace options when you want to **completely hide** the background:

```go
func (m Model) renderMyModal(content string) string {
    modal := m.wrapModal(content, width, height)

    return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, modal,
        lipgloss.WithWhitespaceChars(" "),
        lipgloss.WithWhitespaceForeground(lipgloss.Color("0")))
}
```

**How it works:**
- `lipgloss.Place()` centers the modal and fills surrounding space with spaces
- The spaces use the terminal's default background color (black/0)
- Background content is **hidden**, not dimmed

**Note:** `WithWhitespaceForeground()` sets the foreground color of space characters, which are invisible. This does NOT create visible dimming.

## Adding a New Modal Feature

### 1. Add Fields to ModalEntry

```go
// In model.go, add to ModalEntry struct:
type ModalEntry struct {
    // ... existing fields ...

    // Your feature
    MyFeatureData    []SomeType
    MyFeatureCursor  int
    MyFeatureFocused bool
}
```

### 2. Fetch Data

Update `fetchIssueDetails()` in model.go:

```go
func (m Model) fetchIssueDetails(issueID string) tea.Cmd {
    return func() tea.Msg {
        msg := IssueDetailsMsg{IssueID: issueID}
        // ... existing fetches ...

        // Your feature
        if someCondition {
            msg.MyFeatureData, _ = m.DB.GetMyFeatureData(issueID)
        }
        return msg
    }
}
```

Add field to `IssueDetailsMsg` and handle in `Update()`.

### 3. Add Keymap Context (if needed)

In `keymap/registry.go`:
```go
const (
    ContextMyFeature Context = "my-feature"
)

const (
    CmdMyFeatureAction Command = "my-feature-action"
)
```

In `keymap/bindings.go`:
```go
{Key: "enter", Command: CmdMyFeatureAction, Context: ContextMyFeature},
```

### 4. Update Context Detection

In `currentContext()`:
```go
if m.ModalOpen() {
    if modal := m.CurrentModal(); modal != nil {
        if modal.MyFeatureFocused {
            return keymap.ContextMyFeature
        }
        if modal.TaskSectionFocused {
            return keymap.ContextEpicTasks
        }
    }
    return keymap.ContextModal
}
```

### 5. Handle Commands

In `executeCommand()`:
```go
case keymap.CmdMyFeatureAction:
    if modal := m.CurrentModal(); modal != nil && modal.MyFeatureFocused {
        // Handle action
    }
    return m, nil
```

### 6. Render UI

In `renderModal()`:
```go
// Add section when condition is met
if someCondition && len(modal.MyFeatureData) > 0 {
    header := "MY FEATURE SECTION"
    if modal.MyFeatureFocused {
        header = focusedStyle.Render(header)
    }
    lines = append(lines, header)

    for i, item := range modal.MyFeatureData {
        line := formatItem(item)
        if modal.MyFeatureFocused && i == modal.MyFeatureCursor {
            line = selectedStyle.Render("> " + line)
        }
        lines = append(lines, line)
    }
}
```

## Interactive Modal Buttons

All modals with user actions should use interactive buttons instead of key hints like `[Enter] Confirm [Esc] Cancel`.

### Button Rendering Pattern

```go
// In model struct, add:
buttonFocus int // 0=input, 1=confirm, 2=cancel

// In modal render function:
confirmStyle := styles.Button
cancelStyle := styles.Button
if m.buttonFocus == 1 {
    confirmStyle = styles.ButtonFocused
}
if m.buttonFocus == 2 {
    cancelStyle = styles.ButtonFocused
}

sb.WriteString("\n\n")
sb.WriteString(confirmStyle.Render(" Confirm "))
sb.WriteString("  ")
sb.WriteString(cancelStyle.Render(" Cancel "))
```

### Keyboard Navigation

- **Tab**: Cycle focus between input field and buttons (input → confirm → cancel → input)
- **Shift+Tab**: Reverse cycle
- **Enter**: Execute focused button (or confirm from input)
- **Esc**: Always cancels (global shortcut)

```go
case "tab":
    m.buttonFocus = (m.buttonFocus + 1) % 3
    if m.buttonFocus == 0 {
        m.textInput.Focus()
    } else {
        m.textInput.Blur()
    }
    return m, nil
```

### Mouse Support

Register hit regions for buttons during render:

```go
// Register hit regions (calculate positions based on modal layout)
mouseHandler.AddRect(regionConfirm, x, y, 10, 1, nil)
mouseHandler.AddRect(regionCancel, x+15, y, 10, 1, nil)
```

Handle clicks in the mouse handler:

```go
case regionConfirm:
    return m.executeAction()
case regionCancel:
    return m.cancelAction()
```

### Hover State

Add hover state for visual feedback when mouse moves over buttons:

```go
// In model struct:
buttonHover int // 0=none, 1=confirm, 2=cancel

// Handle hover in mouse handler:
case mouse.ActionHover:
    return m.handleMouseHover(action)

func (m Model) handleMouseHover(action mouse.MouseAction) (Model, tea.Cmd) {
    if action.Region == nil {
        m.buttonHover = 0
        return m, nil
    }
    switch action.Region.ID {
    case regionConfirm:
        m.buttonHover = 1
    case regionCancel:
        m.buttonHover = 2
    default:
        m.buttonHover = 0
    }
    return m, nil
}

// In modal render, focus takes precedence over hover:
confirmStyle := styles.Button
if m.buttonFocus == 1 {
    confirmStyle = styles.ButtonFocused
} else if m.buttonHover == 1 {
    confirmStyle = styles.ButtonHover
}
```

## Core Functions

### renderBaseView()

Renders the panels and footer without any modal overlay. This is the background content used for dimmed modal overlays.

```go
func (m Model) renderBaseView() string {
    // Renders: search bar + Current Work + Task List + Activity + footer
    // Returns the complete base view string
}
```

### OverlayModal()

Composites a modal on top of a dimmed background. Located in `pkg/monitor/overlay.go`.

```go
func OverlayModal(background, modal string, width, height int) string
```

## Visual Indicators

### Border Colors by Depth

| Depth | Color | Code |
|-------|-------|------|
| 1 | Purple/Magenta | `primaryColor` (212) |
| 2 | Cyan | 45 |
| 3+ | Orange | 214 |

### Style Constants

```go
// DimStyle applies dim gray color to background content (overlay.go)
var DimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

// Modal border colors by context (styles.go)
var (
    primaryColor = lipgloss.Color("212") // Purple/Magenta - depth 1 issue modals
    cyanColor    = lipgloss.Color("45")  // Cyan - depth 2+ or form modals
    orangeColor  = lipgloss.Color("214") // Orange - depth 3+
    greenColor   = lipgloss.Color("42")  // Green - handoffs modal
    errorColor   = lipgloss.Color("196") // Red - confirmation dialogs
)

// Styles (styles.go)
epicTasksFocusedStyle  // Cyan, bold - focused section header
epicTaskSelectedStyle  // Inverted - selected item
breadcrumbStyle        // Gray, italic - navigation path
```

## Modal Inventory

| Modal | Border Color | Overlay Type | Render Function |
|-------|--------------|--------------|-----------------|
| Issue details | Purple (depth 1), Cyan (2), Orange (3+) | Dimmed | `renderModal()` |
| Stats | Purple | Dimmed | `renderStatsModal()` |
| Handoffs | Green | Dimmed | `renderHandoffsModal()` |
| Form | Cyan | Dimmed | `renderFormModal()` |
| Confirmation | Red | Dimmed | `renderConfirmation()` |
| Help | N/A (full screen) | N/A | `renderHelp()` |
| TDQ Help | N/A (full screen) | N/A | `renderTDQHelp()` |

## Implementation Checklist

When adding a new modal:

1. **Render the modal content** using appropriate wrapper (`wrapModal()`, `wrapModalWithDepth()`, or inline styling)

2. **Use the dimmed overlay pattern** (preferred):
   ```go
   background := m.renderBaseView()
   modalContent := m.renderMyModal()
   return OverlayModal(background, modalContent, m.Width, m.Height)
   ```

3. **Don't use `lipgloss.Place()` with `OverlayModal()`** - they both handle centering, which causes layout issues.

## Common Pitfalls

1. **Don't use `lipgloss.Place()` with `OverlayModal()`** - they both handle centering, which causes layout issues.

2. **Pass the full background** - `OverlayModal()` needs the complete background content to composite correctly. Don't pre-truncate or pre-dim.

3. **Height constraints** - Ensure modal content respects available height to prevent overflow. Use `wrapModalWithDepth()` for consistent sizing.

4. **Stacked modals** - For stacked issue modals (depth 2+), the background is always the base view (panels), not the previous modal.

## File Locations

- Overlay helper: `pkg/monitor/overlay.go` (`OverlayModal()`, `DimStyle`)
- Base view: `pkg/monitor/view.go` (`renderBaseView()`)
- Modal rendering: `pkg/monitor/view.go` (`renderModal()`, `renderStatsModal()`, etc.)
- Modal logic: `pkg/monitor/modal.go` (stack management, navigation)
- Modal wrapper: `pkg/monitor/view.go` (`wrapModal()`, `wrapModalWithDepth()`)
- Modal styles: `pkg/monitor/styles.go` (border colors, text styles)

## Testing

Add tests in `model_test.go`:

```go
func TestMyFeature(t *testing.T) {
    m := Model{
        Keymap: newTestKeymap(),
        ModalStack: []ModalEntry{{
            IssueID: "td-001",
            MyFeatureData: []SomeType{{}, {}},
            MyFeatureFocused: true,
        }},
    }

    // Test cursor movement, actions, etc.
}
```

## Key Patterns

1. **Always use `CurrentModal()`** - never index `ModalStack` directly
2. **Check `modal != nil`** before accessing fields
3. **Reset focus state** when pushing/popping modals
4. **Update footer help text** in `wrapModalWithDepth()` for new contexts
5. **Add to help.go** for user-visible documentation
