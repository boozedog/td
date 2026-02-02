# E2E Sync Test Suite

Live integration tests that build real `td` + `td-sync` binaries, start a local server, and exercise the full sync flow between two simulated clients (alice and bob).

## Structure

```
scripts/e2e/
  harness.sh        # shared setup/teardown/assertions — source this
  run-all.sh        # runs every test_*.sh, reports pass/fail
  test_*.sh         # individual test scripts
  GUIDE.md          # this file
```

## Running

```bash
bash scripts/e2e/run-all.sh          # run all tests
bash scripts/e2e/run-all.sh --full   # include real-data tests
bash scripts/e2e/test_basic_sync.sh  # run one test
bash scripts/e2e/test_alternating_actions.sh --actions 8  # alternating multi-actor test
bash scripts/e2e/test_chaos_sync.sh --actions 100  # chaos stress test
```

Each test gets its own random port and temp directory. Tests can run sequentially via `run-all.sh` (not parallel — each builds binaries independently).

## Real-data tests (manual)

These depend on local databases and are only run with `--full`:

- `test_sync_real_data.sh` — runs against a single issues DB (default `$HOME/code/td/.todos/issues.db` or a custom path).
- `test_sync_real_data_all_projects.sh` — reads `~/.config/sidecar/config.json` and runs the same test for every project DB it finds.
- `test_monitor_autosync.sh` — verifies that `td monitor`'s periodic auto-sync pushes edits made while the monitor is running. Uses `expect` for pseudo-TTY. Requires `expect` installed.

## Alternating actions test

`test_alternating_actions.sh` alternates Alice/Bob mutations across issues (create → start → log → comment → review → approve) plus board operations, then compares final DB state.

```bash
bash scripts/e2e/test_alternating_actions.sh --actions 6
```

## Chaos sync test

`test_chaos_sync.sh` is a comprehensive stress test that randomly exercises every td mutation type across two syncing clients. It randomly selects from ~28 action types (create, update, delete, status transitions, comments, logs, dependencies, boards, handoffs) with realistic frequency weights, generates arbitrary-length content, and verifies full DB convergence after sync.

```bash
bash scripts/e2e/test_chaos_sync.sh                              # default: 100 actions
bash scripts/e2e/test_chaos_sync.sh --actions 500                # more actions
bash scripts/e2e/test_chaos_sync.sh --duration 60                # run for 60 seconds
bash scripts/e2e/test_chaos_sync.sh --seed 42 --actions 50       # reproducible
bash scripts/e2e/test_chaos_sync.sh --sync-mode aggressive       # sync after every action
bash scripts/e2e/test_chaos_sync.sh --conflict-rate 30 --verbose # 30% simultaneous mutations
```

| Flag | Default | Effect |
|------|---------|--------|
| `--actions N` | `100` | Total actions to perform |
| `--duration N` | — | Run for N seconds (overrides --actions) |
| `--seed N` | `$$` | RANDOM seed for reproducibility |
| `--sync-mode MODE` | `adaptive` | `adaptive` (3-10 action batches), `aggressive` (every action), `random` (25% chance) |
| `--verbose` | off | Print every action detail |
| `--conflict-rate N` | `20` | % of rounds where both clients mutate before syncing |
| `--batch-min N` | `3` | Min actions between syncs (adaptive mode) |
| `--batch-max N` | `10` | Max actions between syncs (adaptive mode) |

The action library lives in `chaos_lib.sh`. Each action type has an `exec_<action>` function that handles preconditions, state tracking, and expected-failure detection.

## Extending the chaos test

When adding new syncable mutations to td, add a corresponding `exec_<action>` function in `chaos_lib.sh` and register it in the `ACTION_WEIGHTS` array. This ensures the new feature gets exercised under randomized multi-client conditions. Follow the existing pattern: check preconditions, run the command, update state tracking, handle expected failures.

## Writing a New Test

Create `scripts/e2e/test_<name>.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/harness.sh"

# 1. Call setup — this builds binaries, starts server, auths alice+bob,
#    creates a project, and links both clients.
setup
# Options:
#   setup --auto-sync                    # enable post-mutation auto-sync
#   setup --auto-sync --debounce "1s"    # custom debounce
#   setup --auto-sync --interval "3s"    # custom periodic interval

# 2. Use td_a / td_b to run commands as alice / bob.
#    Each runs in its own project dir with isolated HOME.
td_a create "My test issue" >/dev/null
td_a sync >/dev/null 2>&1
td_b sync >/dev/null 2>&1

# 3. Query state with td list/show --json and jq.
BOB_LIST=$(td_b list --json 2>/dev/null)
COUNT=$(echo "$BOB_LIST" | jq 'length')

# 4. Assert.
assert_eq "bob sees 1 issue" "$COUNT" "1"

# 5. Always end with report.
report
```

## Harness Reference

### setup options

| Flag | Default | Effect |
|------|---------|--------|
| (none) | — | auto-sync off, explicit `td sync` only |
| `--auto-sync` | — | enable auto-sync (post-mutation push+pull) |
| `--debounce "Xs"` | `"2s"` | min interval between auto-syncs |
| `--interval "Xs"` | `"10s"` | periodic sync interval |

`on_start` is always `false` in tests to avoid debounce interference (startup sync consumes the debounce window, causing the post-mutation sync to be skipped).

### Environment after setup

| Variable | Description |
|----------|-------------|
| `WORKDIR` | Temp dir (cleaned up on exit) |
| `TD_BIN` | Path to built `td` binary |
| `SERVER_URL` | `http://localhost:<random-port>` |
| `PROJECT_ID` | Remote project ID (both clients linked) |
| `SERVER_PID` | Server process ID |
| `CLIENT_A_DIR` | Alice's project directory |
| `CLIENT_B_DIR` | Bob's project directory |
| `HOME_A`, `HOME_B` | Isolated HOME dirs (config + auth) |

### Client helpers

```bash
td_a <args...>   # run td as alice (cd's into CLIENT_A_DIR, sets HOME)
td_b <args...>   # run td as bob
```

### Assertions

```bash
assert_eq "description" "$actual" "$expected"
assert_ge "description" "$actual" "$minimum"
assert_contains "description" "$haystack" "$needle"
assert_json_field "description" "$json" '.jq.expr' "$expected"
```

All assertions increment counters. `report` at the end prints PASS/FAIL with counts. Non-assertion failures use `_fail` (increments failure count) or `_fatal` (exits immediately).

### Polling for async results

For auto-sync tests where you need to wait for propagation:

```bash
# Poll pattern: bob syncs repeatedly until condition is met
TIMEOUT=20
elapsed=0
while [ "$elapsed" -lt "$TIMEOUT" ]; do
    td_b sync >/dev/null 2>&1
    # ... check condition ...
    if [ condition_met ]; then break; fi
    sleep 2
    elapsed=$((elapsed + 2))
done
```

### Logging and debugging

```bash
_step "Description"     # prints section header
_ok "Detail"            # prints green OK line
_fail "Detail"          # prints red FAIL, increments failure count
_fatal "Detail"         # prints red FATAL, exits immediately
```

Server logs are at `$WORKDIR/server.log`. On failure, `report` prints the path.

## Conventions

- File names: `test_<descriptive_name>.sh`
- Use `>/dev/null` or `>/dev/null 2>&1` to suppress td output unless you need it
- Use `--json` output and `jq` for assertions — don't parse human-readable output
- For `td show --json`, logs are in `.logs` (absent when empty, use `.logs // []`)
- For `td list --json`, use `--status all` if testing non-open statuses
- Keep tests focused: one scenario per file
- When testing auto-sync timing, use the polling pattern above with generous timeouts
- When testing multiple mutations, add `sleep` between them to clear the debounce window

## Gotchas

- **Debounce + on_start interaction**: If `on_start` is true, the startup sync of a command (e.g., `td start`) sets `lastAutoSyncAt`, causing the post-mutation sync to be debounced away. The harness sets `on_start: false` to avoid this.
- **td show --json logs field**: Omitted entirely when there are 0 logs (not an empty array). Always use `.logs // []` in jq.
- **Issue IDs**: Extract with `grep -oE 'td-[0-9a-f]+'` from `td create` output (`CREATED td-abc123`).
- **Project IDs**: Extract with `grep -oE 'p_[0-9a-f]+'` from `td sync-project create` output.
