# TD CLI: Dependency Command Structure & Output Formatting Analysis

## Executive Summary

The codebase shows **fragmented output formatting with multiple inconsistent approaches** across commands. While there is a centralized `output/output.go` module with 14 formatting functions, most commands bypass it with direct `fmt.Printf` calls, leading to:

- **Inconsistent symbols** (└──, ├──, ▶, ✓, ⧗, ●, ✗ used across different commands)
- **Uneven spacing and indentation** (no unified style guide)
- **Scattered dependency handling** (3 different patterns for querying dependencies)
- **No shared tree-rendering logic** (tree, blocked-by, depends-on, show --children all implement independently)
- **Color/style applied inconsistently** (some commands use output.FormatStatus, others don't)

---

## Part 1: Dependency Command Structure

### 1.1 Dependency Creation Methods

There are **3 ways** to create dependencies, each with its own code path:

#### Method 1: `td dep [issue] [depends-on-issue]`
**File:** `cmd/dependencies.go:434-498`
```go
var depCmd = &cobra.Command{
    Use: "dep [issue] [depends-on-issue]",
    // Creates dependency via database.AddDependency()
}
```
- **Circular check**: `wouldCreateCycle()` function
- **Output**: `fmt.Printf("ADDED: %s depends on %s\n", ...)`
- **Duplicate check**: Manual loop through `GetDependencies()`

#### Method 2: `td link --depends-on [issue] [target]`
**File:** `cmd/link.go:85-157`
```go
linkCmd with dependsOnID flag
// Same circular check, same AddDependency call
// BUT: Different output format at lines 152-154
fmt.Printf("ADDED: %s depends on %s\n", issue.ID, depIssue.ID)
fmt.Printf("  %s: %s\n", issue.ID, issue.Title)
fmt.Printf("  └── now depends on: %s: %s\n", depIssue.ID, depIssue.Title)
```
- **Code duplication**: Lines 116-157 in link.go duplicates lines 434-498 in dependencies.go
- **Output variance**: Slightly different format (3-line vs 2-line output)

#### Method 3: `td create --depends-on [list]` / `td update --depends-on [list]`
**File:** `cmd/create.go`, `cmd/update.go`
- Handled at parse time, passed to database
- **No user-facing dependency output** in these commands

**PROBLEM**: Dependency creation code is duplicated between `cmd/dep` and `cmd/link.go`.

---

### 1.2 Dependency Query Methods

**3 commands query dependencies; each has unique output formatting:**

#### `td blocked-by [issue-id]`
**File:** `cmd/dependencies.go:13-84`

Output structure:
```
td-5q: Implement OAuth flow [in_progress]
└── blocks:
    ├── td-8c: Add protected routes [open]
    │   └── td-9d: User dashboard [open]
    │   └── td-10e: Admin panel [open]
    └── td-8d: Second level [open]

3 issues blocked (1 direct, 2 transitive)
```

**Key characteristics:**
- Uses `printBlockedTree()` recursive function (lines 86-120)
- Hard-coded tree chars: `"└── blocks:"`, `"├──"`, `"└──"`, `"│   "`
- Indentation: `"    "` (4 spaces) per level
- Status via `output.FormatStatus()` ✓ (consistent with module)
- Tree building happens inside formatting logic

#### `td depends-on [issue-id]`
**File:** `cmd/dependencies.go:139-210`

Output structure:
```
td-8c: Add protected routes [open]
└── depends on:
    td-5q: Implement OAuth flow [in_progress]
    td-2a: Session middleware [closed] ✓

1 blocking, 1 resolved
```

**Key characteristics:**
- **NO tree rendering** (flat list, not hierarchical)
- Uses `output.FormatStatus()` ✓ (consistent)
- Status symbol only for resolved: `" ✓"`
- **Inconsistent indentation**: 4 spaces before issue ID, but no connector chars
- Different structure from `blocked-by` (flat vs tree)

#### `td critical-path`
**File:** `cmd/dependencies.go:212-364`

Output structure (multiple sections):
```
CRITICAL PATH SEQUENCE (resolve in order):

  1. td-5q  Implement OAuth flow  [in_progress]
     └─▶ unblocks 3
  2. td-8c  Add protected routes  [open]
     └─▶ unblocks 2

START NOW (no blockers, unblocks others):
  ▶ td-5q  OAuth flow  (unblocks 6)

BOTTLENECKS (blocking most issues):
  td-5q: 6 issues waiting
  td-3a: 4 issues waiting
```

**Key characteristics:**
- Uses `output.FormatStatus()` ✓
- Mixed ASCII symbols: `└─▶` and `▶` (different from tree)
- Three separate output sections with different formatting
- Numbered list (unusual for this codebase)
- No hierarchical tree rendering

**PROBLEM**: Three different output styles for three dependency-related commands. Symbols and structure all differ.

---

## Part 2: Output Formatting Architecture

### 2.1 Centralized Output Module

**File:** `internal/output/output.go` (300 lines)

**Formatting functions (14 total):**

| Function | Purpose | Used By |
|----------|---------|---------|
| `FormatStatus(status)` | Status with color `[open]` | output.go, dependencies.go, tree.go, show.go, list.go |
| `FormatPriority(priority)` | Priority with color `[P1]` | output.go, show.go, list.go |
| `FormatPoints(points)` | Points suffix `5pts` | output.go, show.go |
| `FormatPointsSuffix()` | Points suffix with spaces | output.go |
| `FormatIssueShort()` | One-line issue format | show.go, list.go |
| `FormatIssueDeleted()` | Deleted issue format `[deleted]` | list.go (only location) |
| `FormatIssueLong()` | Multi-line issue format | show.go, list.go |
| `FormatTimeAgo()` | Relative time `2h ago` | output.go, show.go |
| `ShortSHA()` | Git SHA 7-char | output.go, show.go, link.go |
| `FormatGitState()` | Git state formatting | output.go (never used!) |
| `Success()` | Green success message | Various commands |
| `Error()` | Red error message | Various commands |
| `Warning()` | Yellow warning message | Various commands |
| `Info()` | Plain info message | Various commands |

**Coverage issues:**
- ❌ No function for tree rendering (used 3x independently)
- ❌ No function for dependency output
- ❌ No function for list output styling (partially implemented in FormatIssueShort)
- ❌ `FormatGitState()` defined but never called
- ⚠️  `FormatPoints()` vs `FormatPointsSuffix()` - two almost-identical functions

### 2.2 Direct fmt Usage (Bypassing output module)

**Commands using direct fmt.Printf instead of formatting functions:**

```
cmd/dependencies.go:     24 fmt.Printf calls (64, 77, 79, 90, 111, 113, 178, 181, ...)
cmd/show.go:            12 fmt.Printf calls (154, 157, 160, 161, ...)
cmd/tree.go:             8 fmt.Printf calls (44, 87, 89, 218, ...)
cmd/block.go:            7 fmt.Printf calls
cmd/list.go:             3 fmt.Printf calls (in shortcut commands)
cmd/link.go:             8 fmt.Printf calls (in dependency output)
cmd/context.go:          9 fmt.Printf calls
cmd/ws.go:              15 fmt.Printf calls
cmd/system.go:          12 fmt.Printf calls
cmd/review.go:           6 fmt.Printf calls
```

**Total: ~100+ direct fmt.Printf calls across codebase**

This means **output formatting is fundamentally NOT centralized**.

---

## Part 3: Output Formatting Inconsistencies

### 3.1 Tree Structure Symbols

**Used in different commands:**

| Command | Tree Chars | Indentation | Notes |
|---------|-----------|------------|-------|
| `blocked-by` | `├──`, `└──`, `│   ` | 4 spaces | Proper ASCII tree |
| `tree` | `├──`, `└──`, `│   ` | 4 spaces | Same as blocked-by |
| `show --children` | `├──`, `└──`, none | 2 spaces | Missing vertical lines |
| `depends-on` | `└──` only | 4 spaces | No tree, flat list |
| `critical-path` | `└─▶`, `▶` | 2-3 spaces | Different symbols |

**Inconsistencies:**
1. `show --children` at line 250-256 uses `├──`/`└──` but NO `│` for vertical continuation
2. `blocked-by` and `tree` have proper ASCII trees
3. `critical-path` uses arrows instead (`└─▶`)
4. `depends-on` is flat (no tree structure at all)

### 3.2 Status Indicators

**Different symbols used across commands:**

```
Output Module:
  FormatStatus() → renders [open], [in_progress], etc. with colors

Tree command (tree.go:76-85):
  closed    → " ✓"
  in_review → " ⧗"
  in_progress → " ●"
  blocked   → " ✗"

Show --children (show.go:238-247):
  closed    → " ✓"
  in_review → " ⧗"
  in_progress → " ●"
  blocked   → " ✗"

Dependencies:
  depends-on → " ✓" only (resolved)
  blocked-by → FormatStatus() only (no symbols)
  critical-path → FormatStatus() only (no symbols)
```

**Problem**: Tree-based displays add Unicode symbols AFTER status, but FormatStatus is used as-is by other commands. No unified style.

### 3.3 Spacing Patterns

**No consistent spacing between fields:**

```
FormatIssueShort (output.go:135-148):
  parts = [ID, Priority, Title, Points, Type, Status]
  strings.Join(parts, "  ") → "  " (2 spaces)

blocked-by printing (dependencies.go:111):
  fmt.Printf("%s├── %s: %s %s\n", prefix, issue.ID, issue.Title, status)
  → ID, colon, title, space, status (no fixed spacing)

critical-path (dependencies.go:327):
  fmt.Printf("  %d. %s  %s  %s\n", i+1, id, issue.Title, status)
  → Numbered, then 2 spaces, issue ID, 2 spaces, title, 2 spaces, status

list shorthand (list.go:226):
  fmt.Printf("%s  (impl: %s)\n", output.FormatIssueShort(&issue), implementer)
  → Uses FormatIssueShort, appends metadata
```

**No standard for:**
- Field separator width (1 space, 2 spaces, or custom)
- Metadata ordering
- Alignment/columnar formatting

### 3.4 Indentation Levels

**Varies by context:**

```
Dependency trees:      "    " (4 spaces) per level
Show git state:        "  " (2 spaces) at start
Show handoff:          "  " (2 spaces) for header, "    " (4 spaces) for items
Show linked files:     "  " (2 spaces) at start
Critical path:         "  " (2 spaces) at start, "     " (5 spaces) for sub-bullets
Comments:              None (inline)
```

No centralized indentation utility.

---

## Part 4: Database / Dependency Layer

### 4.1 Dependency Storage

**Single table with dual-purpose design:**

```sql
CREATE TABLE issue_dependencies (
    issue_id TEXT,
    depends_on_id TEXT,
    relation_type TEXT  -- "depends_on" only (blocks stored as reverse)
)
```

**Methods:**
- `AddDependency(issueID, dependsOnID, relationType)` - stores with relation_type
- `GetDependencies(issueID)` - returns IDs this issue depends on
- `GetBlockedBy(issueID)` - returns IDs this issue blocks (by querying depends_on_id column)

**Design note:**
- Only "depends_on" type is stored
- "blocks" is just the reverse query
- No explicit relation_type="blocks" rows (even though parameter accepted)

### 4.2 Cyclic Dependency Prevention

**Implemented identically in 2 places:**

```go
// dependencies.go:501-522
wouldCreateCycle(database, issueID, newDepID)
  ↓
hasCyclePath(database, from, to, visited)

// link.go:116-157
Same wouldCreateCycle() call
```

Same function reused—good! But:
- Code that calls it is duplicated (depCmd and linkCmd)
- No helper for "check if dependency exists" (manual loop both places)

---

## Part 5: Patterns & Anti-Patterns

### ✓ Good Patterns

1. **Centralized database layer** (`internal/db/`) - all queries go through DB struct
2. **Consistent error handling** - `output.Error()` used throughout for error messages
3. **JSON output support** - most commands have `--json` flag
4. **Circular dependency detection** - prevents invalid states

### ✗ Bad Patterns

1. **Output formatting sprawl** - 14 functions in output module, but 100+ direct fmt.Printf calls
2. **Code duplication** - dependency creation code in TWO places (dep.go, link.go lines 116-157)
3. **Inconsistent symbols** - 6 different tree/status symbol sets across commands
4. **No tree-rendering abstraction** - implemented 3+ times independently
5. **Unstructured spacing** - no constants or helpers for indentation/alignment
6. **Inconsistent metadata display** - session info, timestamps, etc. formatted differently everywhere
7. **No style constants** - hardcoded indent `"    "`, tree chars `"├──"` scattered throughout

---

## Part 6: Where Inconsistencies Come From

### Root Causes

1. **No style guide / design system**
   - No "TD CLI Style Specification" (SPEC.md covers commands, not formatting)
   - Each command author made independent choices

2. **Output module incomplete**
   - Started with basic functions (FormatStatus, FormatPriority)
   - Never expanded to cover dependency/tree rendering
   - Tree formatting needs are different (requires recursive context)

3. **Git commit history suggests organic growth**
   - Commands added incrementally
   - Output formatting not refactored as codebase grew
   - Each new command copied closest precedent

4. **Two classes of commands**
   - **Mutation** (`create`, `start`, `review`) → use output.Success/Error
   - **Query** (`list`, `show`, `tree`) → mix of output module + fmt.Printf

5. **Dependency graph added later?**
   - `blocked-by`, `depends-on`, `critical-path` all in one file
   - Similar patterns but inconsistent implementations
   - Suggests added by multiple people/at different times

---

## Architecture Summary Diagram

```
┌─────────────────────────────────────────────────────────────┐
│  Dependency Commands                                        │
├─────────────────────────────────────────────────────────────┤
│  td dep           → dependencies.go:434-498                 │
│  td link --dep    → link.go:116-157 (DUPLICATED CODE)      │
│  td blocked-by    → dependencies.go:13-84                  │
│  td depends-on    → dependencies.go:139-210                │
│  td critical-path → dependencies.go:212-364                │
│                                                             │
│  Create/Update:   → create.go/update.go (no display)        │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│  Database Layer (internal/db/)                              │
├─────────────────────────────────────────────────────────────┤
│  AddDependency()     │ GetDependencies()  │ GetBlockedBy()   │
│  RemoveDependency()  │                    │                  │
└─────────────────────────────────────────────────────────────┘
         ↓
┌─────────────────────────────────────────────────────────────┐
│  SQLite (issue_dependencies table)                          │
│  [issue_id] [depends_on_id] [relation_type]                 │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Output Formatting                                          │
├─────────────────────────────────────────────────────────────┤
│  CENTRALIZED (output/output.go)                             │
│  ├─ FormatStatus()      ✓ Used: deps, show, list           │
│  ├─ FormatPriority()    ✓ Used: show, list                 │
│  ├─ FormatPoints()      ✓ Used: show                        │
│  ├─ FormatIssueShort()  ✓ Used: show, list                 │
│  ├─ FormatIssueLong()   ✓ Used: show, list                 │
│  └─ FormatTimeAgo()     ✓ Used: show                        │
│                                                             │
│  SCATTERED (cmd/*.go files)                                │
│  ├─ Tree rendering      - blocked-by, tree, show           │
│  ├─ Status symbols      - tree, show --children            │
│  ├─ Indentation logic   - Every file independently         │
│  ├─ Git state display   - show.go custom                   │
│  ├─ Handoff display     - output.go but needs refactoring  │
│  ├─ Session display     - show.go custom                   │
│  └─ List formatting     - 5+ different list commands       │
└─────────────────────────────────────────────────────────────┘
```

---

## Recommendations

### Tier 1: Critical (Reduce Duplication)

1. **Remove dependency creation duplication**
   - Move both `dep` and `link --depends-on` to unified function
   - Share code path (currently in dependencies.go + link.go)

2. **Extract tree rendering to output module**
   ```go
   // New in output/output.go:
   RenderTree(issue, children, indent, symbols) string
   RenderBlockingTree(db, issueID, directOnly) string
   ```

3. **Define style constants**
   ```go
   const (
       IndentLevel = 4  // or "\t"
       TreePrefix  = "├── "
       TreeLast    = "└── "
       TreeVert    = "│   "
   )
   ```

### Tier 2: Consistency (Standardize Formatting)

4. **Unify status display** - decide on one approach:
   - Option A: `[open]` (colored, no symbols)
   - Option B: `[open] ●` (colored with status symbol)
   - Apply everywhere

5. **Create formatting style guide** - document in comments:
   - Spacing rules (2 spaces between fields)
   - Indentation (4 spaces per level)
   - Tree symbols (pick one set)
   - Metadata ordering

6. **Add spacing/alignment utilities**
   ```go
   FormatColumns(fields []string, widths []int) string
   FormatIndent(level int, content string) string
   ```

### Tier 3: Long-term Architecture

7. **Consider renderer interface**
   ```go
   type IssueRenderer interface {
       Render(*Issue) string
   }
   // Different implementations: ShortRenderer, LongRenderer, TreeRenderer
   ```

8. **Expand output module scope** - rename to `formatting/` or `display/` to reflect bigger role

9. **Document output decisions** - SPEC.md sections on formatting per command type

---

## Files Affected by Inconsistencies

**Most problematic:**
- `cmd/dependencies.go` - 3 different output styles, code duplication with link.go
- `cmd/show.go` - 150+ lines of custom formatting
- `cmd/link.go` - duplicates dependency code from dependencies.go
- `cmd/tree.go` - independent tree rendering
- `cmd/context.go` - custom formatting scattered
- `cmd/ws.go` - complex custom output sections

**Moderately problematic:**
- `cmd/list.go` - relies on output.FormatIssueShort (good) but has custom shorthand commands
- `cmd/system.go` - custom stats display
- `cmd/review.go` - custom output

**Well-structured:**
- `cmd/create.go`, `cmd/update.go`, `cmd/delete.go` - use output.Success/Error correctly
- `internal/output/output.go` - good base, just incomplete

---

## Summary Table

| Aspect | Status | Details |
|--------|--------|---------|
| **Centralized formatting** | ⚠️ Partial | Module exists but incomplete; 100+ fmt.Printf bypass it |
| **Dependency commands** | ❌ Inconsistent | 3 display styles, 2 creation paths, code duplication |
| **Tree rendering** | ❌ Fragmented | 3+ independent implementations with different symbols |
| **Color/styling** | ⚠️ Inconsistent | FormatStatus used sometimes, other times hardcoded colors |
| **Spacing** | ❌ Chaotic | No constants, 1-5 space variations, no alignment |
| **Status symbols** | ❌ Fragmented | 6 different symbol sets (✓, ⧗, ●, ✗, └─▶, ▶) |
| **Code duplication** | ⚠️ Medium | Dependency code duplicated (dep.go vs link.go) |
| **Database layer** | ✓ Good | Consistent, but dual-purpose table design |

