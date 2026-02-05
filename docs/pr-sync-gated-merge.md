# PR Title
`merge: integrate sunc behind default-off feature flags`

# PR Body
## Summary
This PR brings the `sunc` sync implementation into `main` while keeping end-user sync behavior disabled by default.

It introduces a feature-flag framework and wires sync surfaces to explicit gates so we can:
- ship DB/schema migrations early,
- continue internal sync testing on mainline,
- avoid exposing auth/sync CLI and background network behavior to all users yet.

## What Changed
### 1) Merged sync stack from `sunc`
- Sync engine, API/server, client commands, migrations, e2e/syncharness coverage, docs, and deployment assets.

### 2) Added feature-flag framework
- New registry + resolution logic:
  - `internal/features/features.go`
  - `internal/features/features_test.go`
- Gate map for sync touchpoints:
  - `internal/features/sync_gate_map.go`
- Local config persistence for flags:
  - `internal/models/models.go` (`feature_flags`)
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- CLI for management:
  - `cmd/feature.go`
  - `cmd/feature_test.go`

### 3) Gated sync command registration (`sync_cli`, default: off)
- `cmd/auth.go`
- `cmd/sync.go`
- `cmd/project.go` (`sync-project`)
- `cmd/config.go` (sync config surface)
- `cmd/doctor.go`

### 4) Gated autosync hooks (`sync_autosync`, default: off)
- Root lifecycle hook framework:
  - `cmd/feature_gate.go`
  - `cmd/root.go`
- Autosync hooks registered via:
  - `cmd/autosync.go`
- Monitor periodic autosync path gated in:
  - `cmd/monitor.go`

### 5) Gated monitor sync prompt (`sync_monitor_prompt`, default: off)
- `pkg/monitor/commands.go` (`checkSyncPrompt` now gated with `BaseDir` context)

### 6) Test harness compatibility
- `test/e2e/harness.go` enables sync flags in test subprocess env so e2e coverage remains valid with production defaults off.

## Feature Flags
- `sync_cli` (default `false`)
- `sync_autosync` (default `false`)
- `sync_monitor_prompt` (default `false`)

Resolution priority:
1. Env overrides (`TD_FEATURE_*`, `TD_ENABLE_FEATURE`, `TD_DISABLE_FEATURE`)
2. Local project config (`.todos/config.json` -> `feature_flags`)
3. Code default

Emergency kill switch:
- `TD_DISABLE_EXPERIMENTAL=1`

## Validation
Executed:
- `go test ./internal/features ./cmd ./pkg/monitor`
- `go test ./test/e2e`
- `go test ./...`

All passed.

## Rollout Plan
1. Release with all sync flags default-off.
2. Monitor migration outcomes and support signals.
3. Enable flags for internal canary cohorts only.
4. Roll out progressively (`sync_cli` -> `sync_monitor_prompt` -> `sync_autosync`).

## Rollback Plan
If needed, disable all experimental behavior immediately with:
- `TD_DISABLE_EXPERIMENTAL=1`

## Notes
- Existing local `CLAUDE.md` changes were intentionally left untouched.
