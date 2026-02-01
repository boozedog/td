#!/usr/bin/env bash
#
# Test: alternating multi-actor mutations with convergence at end.
# Alice and Bob alternately create issues, transition them, comment, add logs,
# create boards, and close reviewed issues. Final DB state must match.
#
set -euo pipefail
source "$(dirname "$0")/harness.sh"

ROUNDS="${ACTIONS:-6}"
BOARDS_EVERY=3

usage() {
    cat <<EOF
Usage: bash scripts/e2e/test_alternating_actions.sh [--actions N] [--boards-every N]

Each round produces multiple mutations (create, start, log, comment, review, approve, board ops).
Defaults: --actions 6, --boards-every 3
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --actions) ROUNDS="$2"; shift 2 ;;
        --boards-every) BOARDS_EVERY="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown arg: $1" >&2; usage; exit 1 ;;
    esac
done

setup

run_td() {
    local who="$1"; shift
    if [ "$who" = "a" ]; then
        td_a "$@"
    else
        td_b "$@"
    fi
}

name_for() {
    if [ "$1" = "a" ]; then
        echo "alice"
    else
        echo "bob"
    fi
}

td_a sync >/dev/null 2>&1 || true
td_b sync >/dev/null 2>&1 || true

_step "Alternating actions (rounds: $ROUNDS)"
for i in $(seq 1 "$ROUNDS"); do
    if (( i % 2 == 1 )); then
        actor="a"
        other="b"
    else
        actor="b"
        other="a"
    fi

    actor_name=$(name_for "$actor")
    other_name=$(name_for "$other")

    create_out=$(run_td "$actor" create "Alt round $i from $actor_name" 2>&1)
    issue_id=$(echo "$create_out" | grep -oE 'td-[0-9a-f]+' | head -n1)
    [ -n "$issue_id" ] || _fatal "Create failed (round $i): $create_out"
    _ok "Created $issue_id by $actor_name"

    run_td "$actor" start "$issue_id" --reason "round $i start" >/dev/null
    run_td "$actor" log --issue "$issue_id" "progress note $i" >/dev/null
    run_td "$actor" log --issue "$issue_id" --hypothesis "hypothesis $i" >/dev/null
    run_td "$actor" comments add "$issue_id" "creator comment $i" >/dev/null

    sleep 1
    run_td "$actor" review "$issue_id" --reason "ready for review $i" >/dev/null

    run_td "$actor" sync >/dev/null 2>&1
    run_td "$other" sync >/dev/null 2>&1

    run_td "$other" comments add "$issue_id" "reviewer comment $i" >/dev/null
    run_td "$other" approve "$issue_id" --reason "approved by $other_name $i" >/dev/null

    if (( i % BOARDS_EVERY == 1 )); then
        board_name="Round-$i Board"
        run_td "$actor" board create "$board_name" >/dev/null
        run_td "$actor" board move "$board_name" "$issue_id" 1 >/dev/null
        run_td "$actor" board edit "$board_name" -q "status != closed" >/dev/null || true
    fi

    run_td "$other" sync >/dev/null 2>&1
    run_td "$actor" sync >/dev/null 2>&1
done

_step "Final convergence"
td_a sync >/dev/null 2>&1
td_b sync >/dev/null 2>&1

DB_A="$CLIENT_A_DIR/.todos/issues.db"
DB_B="$CLIENT_B_DIR/.todos/issues.db"

ISSUE_IDS_A=$(sqlite3 "$DB_A" "SELECT id FROM issues WHERE deleted_at IS NULL ORDER BY id;")
ISSUE_IDS_B=$(sqlite3 "$DB_B" "SELECT id FROM issues WHERE deleted_at IS NULL ORDER BY id;")
assert_eq "issue IDs match" "$ISSUE_IDS_A" "$ISSUE_IDS_B"

ISSUE_STATUS_A=$(sqlite3 "$DB_A" "SELECT id || ':' || status FROM issues WHERE deleted_at IS NULL ORDER BY id;")
ISSUE_STATUS_B=$(sqlite3 "$DB_B" "SELECT id || ':' || status FROM issues WHERE deleted_at IS NULL ORDER BY id;")
assert_eq "issue statuses match" "$ISSUE_STATUS_A" "$ISSUE_STATUS_B"

COMMENT_ROWS_A=$(sqlite3 "$DB_A" "SELECT issue_id || ':' || text || ':' || session_id FROM comments ORDER BY issue_id, id;")
COMMENT_ROWS_B=$(sqlite3 "$DB_B" "SELECT issue_id || ':' || text || ':' || session_id FROM comments ORDER BY issue_id, id;")
assert_eq "comments match" "$COMMENT_ROWS_A" "$COMMENT_ROWS_B"

LOG_ROWS_A=$(sqlite3 "$DB_A" "SELECT issue_id || ':' || type || ':' || message || ':' || session_id FROM logs ORDER BY issue_id, id;")
LOG_ROWS_B=$(sqlite3 "$DB_B" "SELECT issue_id || ':' || type || ':' || message || ':' || session_id FROM logs ORDER BY issue_id, id;")
assert_eq "logs match" "$LOG_ROWS_A" "$LOG_ROWS_B"

BOARD_ROWS_A=$(sqlite3 "$DB_A" "SELECT name || ':' || query || ':' || is_builtin FROM boards ORDER BY name;")
BOARD_ROWS_B=$(sqlite3 "$DB_B" "SELECT name || ':' || query || ':' || is_builtin FROM boards ORDER BY name;")
assert_eq "boards match" "$BOARD_ROWS_A" "$BOARD_ROWS_B"

BOARD_POS_A=$(sqlite3 "$DB_A" "SELECT board_id || ':' || issue_id || ':' || position FROM board_issue_positions ORDER BY board_id, position;")
BOARD_POS_B=$(sqlite3 "$DB_B" "SELECT board_id || ':' || issue_id || ':' || position FROM board_issue_positions ORDER BY board_id, position;")
assert_eq "board positions match" "$BOARD_POS_A" "$BOARD_POS_B"

report
