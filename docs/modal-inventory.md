# Modal Inventory and Compliance

This document provides a comprehensive inventory of all modals in the TD monitor.

**New Implementation**: Use the declarative modal library documented in [declarative-modal-guide.md](guides/declarative-modal-guide.md).

**Legacy Reference**: The original guide is at [modal-system-guide.md](guides/modal-system-guide.md) (deprecated).

## Compliance Features

| Feature | Description |
|---------|-------------|
| ModalStack | Uses stack-based architecture for nested modals |
| OverlayModal | Uses dimmed background overlay via OverlayModal() |
| Depth Colors | Border color changes by depth (purple/cyan/orange) |
| Keyboard Nav | Tab, Shift+Tab, arrow keys, Enter, Esc support |
| Mouse Click | Left-click handler for interactions |
| Mouse Hover | Hover state tracking and visual feedback |
| Mouse Scroll | Mouse wheel scroll wheel support |
| Interactive Buttons | Uses styled buttons instead of key hints |
| Context | Dedicated keymap context for keybindings |
| Commands | Handles commands in executeCommand() |
| Scrollable | Can scroll if content exceeds available height |
| Help Text | Shows keybindings/help in footer |

## Modal Compliance Matrix

| Modal | Purpose | ModalStack | OverlayModal | Depth Colors | Keyboard Nav | Mouse Click | Mouse Hover | Mouse Scroll | Interactive Buttons | Context | Commands | Scrollable | Help Text |
|-------|---------|:----------:|:------------:|:------------:|:-------------:|:----------:|:----------:|:----------:|:------------------:|:-------:|:--------:|:----------:|:---------:|
| Issue Details | View/interact with issue, navigate dependencies | YES | YES | YES | YES | YES | YES | YES | YES | YES | YES | YES | YES |
| Statistics | Show project stats, status/type/priority breakdown | NO | YES | NO | YES | NO | NO | YES | NO | NO | NO | YES | YES |
| Handoffs | View recent handoffs, open issue from list | NO | YES | NO | YES | NO | NO | YES | NO | NO | NO | NO | YES |
| Form Modal | Create/edit issues, inline form with fields | NO | YES | NO | YES | YES | YES | NO | YES | YES | YES | NO | YES |
| Delete Confirmation | Confirm destructive delete action | NO | YES | NO | YES | YES | YES | NO | YES | NO | YES | NO | YES |
| Close Confirmation | Confirm close with optional reason text | NO | YES | NO | YES | YES | YES | NO | YES | NO | YES | NO | YES |
| Board Picker | Select board for new issue | NO | YES | NO | YES | YES | YES | YES | NO | NO | NO | NO | YES |
| Help Modal | Show keybindings and navigation help | NO | YES | NO | YES | NO | NO | YES | NO | NO | NO | YES | N/A |
| TDQ Help | Show query language syntax | NO | NO | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A | NO | N/A |

## Detailed Compliance Analysis

### Issue Details Modal (FULLY COMPLIANT)
**Location**: `pkg/monitor/view.go:958-1268`, `renderModal()`

**Status**: Exceeds guide requirements
- Uses full ModalStack architecture with depth-aware styling
- All keyboard navigation (↑↓ scroll, Tab focus, Enter select, Esc close)
- Complete mouse support (click, hover, scroll)
- Interactive breadcrumb and section headers
- Multiple focusable sections (main content, task list, blocked-by, blocks)
- Proper context detection for keybindings
- Full command handling for all interactions
- Footer help text with keybindings

**Non-Conformances**: None

---

### Statistics Modal (PARTIALLY COMPLIANT)
**Location**: `pkg/monitor/view.go:1294-1440`, `renderStatsModal()`

**Status**: Missing interactive button pattern
- Uses OverlayModal with dimmed background
- Keyboard navigation: ↑↓ scroll, Esc close
- Mouse scroll wheel support
- Scrollable content with scroll clamping
- Footer help text

**Non-Conformances**:
- No interactive buttons (uses text hints instead: "Press esc to close")
- No dedicated context detection
- No command handling in executeCommand()
- No mouse click or hover handlers
- Fixed border color (green), no depth styling

**Recommendation**: Add interactive button pattern for consistency, though scrolling-only modal is acceptable with key hints.

---

### Handoffs Modal (PARTIALLY COMPLIANT)
**Location**: `pkg/monitor/view.go:1443-1549`, `renderHandoffsModal()`

**Status**: Basic implementation, missing advanced features
- Uses OverlayModal with dimmed background
- Keyboard navigation: ↑↓ select, Enter open issue
- Mouse scroll wheel support
- Selectable rows with cursor
- Footer help text

**Non-Conformances**:
- No interactive buttons (cursor-based selection only)
- No dedicated context detection for modal
- No mouse click or hover handlers
- Fixed border color (green), no depth styling
- Uses text hints ("Press esc to close") instead of interactive elements

**Recommendation**: Add mouse click handler to select handoffs, add hover state for better UX.

---

### Form Modal (COMPLIANT)
**Location**: `pkg/monitor/view.go:1670-1723`, `renderFormModal()`

**Status**: Compliant with appropriate adaptations
- Uses OverlayModal with dimmed background
- Keyboard navigation: Tab/Shift+Tab between fields, Ctrl+S submit, Esc cancel
- Mouse support: via huh library (click, focus management)
- Interactive Submit/Cancel buttons
- Custom context ("FormInput" fields in form library)
- Command handling for Ctrl+S, Ctrl+X, Esc
- Footer help text
- Fixed border color (cyan) appropriate for form type

**Non-Conformances**: None - form library provides interactive UI

---

### Delete Confirmation Modal (COMPLIANT)
**Location**: `pkg/monitor/view.go:1928-1978`, `renderConfirmation()`

**Status**: Fully compliant with confirmation pattern
- Uses OverlayModal with dimmed background
- Keyboard navigation: Tab to cycle buttons, Y/N quick keys, Enter confirm, Esc cancel
- Mouse support: click buttons, hover state tracking
- Interactive Yes/No buttons with hover/focus styling
- Fixed border color (red) appropriate for destructive action
- Command handling via Tab/Shift+Tab and Y/N
- Footer help text showing available keys

**Non-Conformances**: None

**Notes**: No dedicated context needed - uses generic ConfirmOpen flag with button focus state.

---

### Close Confirmation Modal (COMPLIANT)
**Location**: `pkg/monitor/view.go:1981-2038`, `renderCloseConfirmation()`

**Status**: Fully compliant with confirmation pattern
- Uses OverlayModal with dimmed background
- Keyboard navigation: Tab cycle (input → Confirm → Cancel → input), Enter confirm, Esc cancel
- Mouse support: click buttons, hover state tracking
- Text input field + interactive Confirm/Cancel buttons
- Interactive button styling with hover/focus
- Fixed border color (red) appropriate for state change
- Command handling via Tab/Shift+Tab and Enter/Esc
- Footer help text showing available keys

**Non-Conformances**: None

**Notes**: Properly manages focus between text input and buttons.

---

### Board Picker Modal (MOSTLY COMPLIANT)
**Location**: `pkg/monitor/view.go:1578-1667`, `renderBoardPicker()`

**Status**: Compliant with minor gaps
- Uses OverlayModal with dimmed background
- Keyboard navigation: ↑↓ select, Enter confirm
- Mouse support: click items, hover state tracking, scroll wheel
- Selectable rows with cursor highlighting
- Mouse hover state rendering
- Fixed border color (purple)
- Footer help text

**Non-Conformances**:
- No interactive button pair (uses cursor navigation only)
- No dedicated context detection
- No command handling in executeCommand() (handled via switch on msg.Type)

**Notes**: Cursor-based selection is appropriate for picker UI. Guide recommends buttons, but this approach is acceptable for list selection.

---

### Help Modal (N/A - SPECIAL CASE)
**Location**: `pkg/monitor/view.go:2194-2283`, `renderHelp()`

**Status**: Not subject to standard compliance - full-screen overlay
- Full terminal overlay (not centered modal)
- Keyboard navigation: j/k line scroll, Ctrl+d/u half-page, G/gg jump to ends, Page Up/Down
- Mouse scroll wheel support
- Scrollable content with scroll indicators (▲/▼)
- Border styling (purple)
- Scroll hints in footer

**Notes**: Help modal is full-screen by design. OverlayModal used with full background content.

---

### TDQ Help Overlay (N/A - SPECIAL CASE)
**Location**: `pkg/monitor/view.go:2286-2288`, `renderTDQHelp()`

**Status**: Not a traditional modal - overlay text
- Simple text overlay showing query syntax
- Generated on demand when search mode active
- Uses OverlayModal for display

**Notes**: This is minimal help text, not a full modal. No interaction needed.

---

## Patterns and Anti-Patterns

### Good Patterns Observed

1. **Consistent OverlayModal Usage** (8/9 modals)
   - All primary modals use OverlayModal for dimmed background overlay
   - Provides visual focus and context preservation

2. **Comprehensive Keyboard Navigation**
   - Most modals support multiple keyboard inputs
   - Tab/Shift+Tab for focus cycling
   - Esc consistently closes

3. **Mouse Support Hierarchy**
   - Scroll wheel: 7/9 modals
   - Click handlers: 6/9 modals
   - Hover state: 5/9 modals

4. **Interactive Buttons**
   - Confirmation modals use styled button pairs
   - Form modal uses huh library UI

### Anti-Patterns and Gaps

1. **Text Hints Instead of Buttons**
   - Stats modal: "Press esc to close"
   - Handoffs modal: "Press esc to close"
   - Should use interactive button pattern

2. **Inconsistent Context Detection**
   - Issue details: Full context detection
   - Stats/Handoffs: No context detection
   - Confirmation: Uses boolean flags, not context

3. **Missing Click Handlers**
   - Stats modal: No click handler (scrolling only)
   - Handoffs modal: No click handler despite being selectable
   - Should enable clicking on rows

4. **Help Text Placement**
   - Most use footer area (appropriate)
   - Some use subtle text hints (inconsistent)

---

## Recommendations for Improvements

### High Priority

1. **Add click handlers to Stats and Handoffs modals**
   - Allow clicking to interact with content
   - Pattern already exists in Board Picker

2. **Replace text hints with interactive buttons**
   - Stats modal should have visible close button
   - Handoffs modal should enable row selection via click

3. **Add missing hover states**
   - Stats modal rows should be hoverable
   - Handoffs modal rows should be hoverable

### Medium Priority

1. **Consolidate context detection**
   - Create consistent pattern for all modals
   - Use currentContext() for all modal types

2. **Standardize command handling**
   - Move all modal handling to executeCommand()
   - Reduce special-case checks in input.go

3. **Add custom renderer vertical padding checks**
   - Ensure modal bounds calculations account for custom renderers
   - Document embedded mode requirements (per guide section)

### Low Priority

1. **Visual consistency**
   - Consider unified help text styling
   - Align footer text placement across all modals

2. **Documentation**
   - Update help.go when adding new modal features
   - Maintain this inventory as features evolve

---

## Testing Gaps

Current modal implementations should verify:

- [ ] Issue Details: All keyboard navigation, depth colors change correctly, mouse click on sections
- [ ] Statistics: Scroll clamping, mouse wheel at edges, ESC closes
- [ ] Handoffs: Cursor stays in bounds, ESC closes, Enter opens issue
- [ ] Form: All field types interactive, Tab cycles focus, Ctrl+S submits
- [ ] Delete Confirmation: Tab cycles buttons, Y/N quick keys, hover states
- [ ] Close Confirmation: Input field focus, button cycling, reason text preserved
- [ ] Board Picker: Mouse hover tracking, scroll wrapping, click selects
- [ ] Help: Scroll boundaries (G/gg), scroll indicator display
- [ ] TDQ Help: Appears/disappears correctly, text readable

---

## File Locations

All implementations located in `pkg/monitor/`:
- **Modal state**: `types.go` (ModalEntry struct, Model fields)
- **Stack management**: `modal.go` (push/pop/navigate functions)
- **Rendering**: `view.go` (all render functions)
- **Keyboard/Mouse**: `input.go` (all handlers)
- **Form dimensions**: `form_modal.go`
- **Overlay compositing**: `overlay.go`
- **Keybindings**: `keymap/registry.go`, `keymap/bindings.go`
