---
type: Task
title: T3 State and apply driver
description: Resume cursor and pipeline orchestration.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m3
---

# Goal

State + apply driver with fake steps.

# Steps order

`resolve` runs before the pipeline (pre-pipeline planning). The pipeline itself
persists resume state for four steps:

`packages` → `tools` → `dotfiles` → `hooks`

# Tests first

- `TestState_roundTrip` — save and load state preserves fields.
- `TestState_resetOnSelectionChange` — different fingerprint clears completed/current/error.
- `TestApply_continuesAfterFailure` — resumes from the failed step.
- `TestApply_successRerunsFullPipeline` — completed run reruns all steps.
- `TestStepOrder` — steps run in packages → tools → dotfiles → hooks order.
- `TestApply_noResumeFlag` — no `--resume` in CLI or API.
- `TestFileStore_sidecarLockMutualExclusion` / `TestFileStore_lockSurvivesRename` / `TestFileStore_lockBlocksUntilUnlock` — the sidecar lock serializes openers and survives atomic rename.
- `TestApply_secondApplyBlocksOnSidecarLock` — a second apply blocks on the sidecar lock while the first apply is mid-pipeline.
- `TestState_loadCorruptReturnsError` — corrupt state error includes a recovery hint naming the file.
- `TestProfileStatePath_resolvesSymlinks` — a profile reached via a symlink shares the canonical state path.
- `TestDefaultPath_noHomeReturnsError` — missing XDG_STATE_HOME and home dir is an explicit error.

# Implementation notes

- `internal/state` persists JSON state under the XDG state directory (`$XDG_STATE_HOME/dotdrift/`, defaulting to `~/.local/state/dotdrift/`). The path is profile-specific so switching profiles does not collide resume state. `--state` still overrides the path.
- State fields: `selection` (fingerprint), `completed` (map of step names), `current` (step name), `status` (enum), `error` (string).
- `internal/apply` orchestrates the pipeline. Each step is an interface (`Step`) with `Run(ctx) error`.
- The apply driver:
  1. Resolves the plan (pre-pipeline).
  2. Compares selection fingerprint to persisted state; if different, resets state.
  3. Runs steps in order, skipping already-completed steps on resume.
  4. On failure, sets `current` and `error` and returns the error.
  5. On success, marks all steps completed.
- Steps are backed by fakes in tests; no real backends.
- Concurrency (decision D1a): `FileStore.Lock` takes `flock(LOCK_EX)` on the sidecar `<state-path>.lock`, never on the state file itself. `cmd/apply` holds it across the entire load→pipeline→save window (`defer store.Unlock()`), so concurrent `dotdrift apply` invocations serialize. The sidecar path is stable across `Save`'s atomic tmp+rename, so the lock is never orphaned on an unlinked inode. `Load`/`Save` do not lock internally; `Save` stays atomic (tmp file, fsync, rename) so lock-free readers (e.g. `status`) never see a torn write.

# Acceptance

- No `--resume` flag.
- State fields only: selection, completed, current, status, error.
- Fresh, mid-fail, success, and selection-change behaviors are tested.
