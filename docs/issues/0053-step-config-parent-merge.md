---
type: Issue
title: Step configs merge the parent snapshot — system entries leak into the user dotfiles step
description: mise --cd config discovery walks up and merges the ancestor <state>/mise/mise.toml (full user+system snapshot) into every per-step config, so the user dotfiles step applies /etc entries as the user; on fresh systems the pipeline dies before the elevating step runs.
tags: [issue, mise, dotfiles, scope, dogfooded]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0053: Step configs merge the parent snapshot

- **Type**: bug
- **Status**: done
- **Priority**: critical
- **Labels**: [mise, dotfiles, scope, dogfooded]
- **Assignee**: agent
- **Related**: [0042](0042-system-files-bootstrap-files.md), [0046](0046-apply-force-flag.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go)
- **Closing commits**: f82ba97

## Summary

Field report (fresh system): `apply` dies in **step `dotfiles`** with
`mise ERROR failed copy: … -> /etc/boot/hooks/post.d/91-… Permission denied`,
and no sudo is ever triggered. Root cause: the D8a full-plan snapshot lives at
`<state>/mise/mise.toml` and every per-step config lives one level below
(`<state>/mise/<step>/mise.toml`). `mise dotfiles apply --cd <step-dir>`
discovers configs from `--cd` **upward** (verified against mise 2026.9.1:
`--cd` is additive, hierarchy merges), so the user step's apply merges the
parent snapshot's `[dotfiles]` — including system-scope `/etc` copy entries —
even though dotdrift's scope split excluded them from the step's own config.

## Details

Why it was invisible until now:

- The scope-splitting pre-flight (`systemTargetsUserWritable`) sees only the
  step's own entries — all user-writable — so **no sudo**, and the pipeline
  dies in the user step *before* `dotfiles-system` (the step that elevates)
  ever runs. Consistent with the field report ("sudo is not triggered").
- On converged systems every merged entry is already up to date → no writes
  → no error. E2E runs as root → writes succeed. Only a **non-root cold run
  with system-scope entries** (a fresh system) trips it.
- Affected invocations: both `mise dotfiles apply` paths (user `dotfiles`
  step, `system-edits` step). The edits step additionally merges the
  snapshot's whole-file entries with their *original* modes (symlink!) into
  an elevated apply. Bootstrap phases (`--only packages|files|…`) are
  unaffected — the snapshot carries no `[bootstrap.*]` sections.

Fix: move the snapshot to a sibling subtree (`<state>/mise/shared/mise.toml`)
so no step config has an ancestor `mise.toml`; hooks steps `--cd` into
`shared/` (their `[tasks]` live there). Layout regression test: after any
apply, no written step config has an ancestor `mise.toml` below the state
root.

## Acceptance Criteria

- [x] Snapshot lives at `<state>/mise/shared/mise.toml`; hooks steps use it from there
- [x] No step config (`tools`, `dotfiles`, `packages`, `system`, `system-edits`, `systemd`, `mounts`, `smb`) has an ancestor `mise.toml` below the state root (layout test)
- [x] Existing snapshot/clobber tests updated to the new path; suite green

## Out of Scope

- Asking mise for a single-file config flag (none exists for
  `dotfiles apply`/`bootstrap` — verified `--help`).
- v0.30.x backporting; the fix ships in the next release.

## Notes

Empirical repro (mise 2026.9.1): parent `mise/mise.toml` with a `/etc` copy
entry + child `mise/dotfiles/mise.toml` without it →
`mise dotfiles apply --cd mise/dotfiles --dry-run` attempts the `/etc` copy.
