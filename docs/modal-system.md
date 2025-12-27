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

## Visual Indicators

### Border Colors by Depth

| Depth | Color | Code |
|-------|-------|------|
| 1 | Purple/Magenta | `primaryColor` (212) |
| 2 | Cyan | 45 |
| 3+ | Orange | 214 |

### Styles (styles.go)

```go
epicTasksFocusedStyle  // Cyan, bold - focused section header
epicTaskSelectedStyle  // Inverted - selected item
breadcrumbStyle        // Gray, italic - navigation path
```

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
