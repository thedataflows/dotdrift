---
type: Issue
title: apply --force for pre-existing files at managed targets
description: mise dotfiles apply refuses to overwrite a regular file sitting at a managed symlink target; dotdrift had no way to forward --force, so app-written live configs aborted the whole apply.
tags: [issue, mise, dotfiles, cli]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0046: apply --force for pre-existing files at managed targets

- **Type**: feature
- **Status**: done
- **Priority**: high
- **Labels**: [mise, dotfiles, dogfooded]
- **Assignee**: agent
- **Related**: [contract invariant 3](../product/contract.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), [`internal/mise/step.go`](../../internal/mise/step.go), [`internal/mise/mise.go`](../../internal/mise/mise.go)
- **Closing commits**: 09cdd5c

## Summary

Field report: `mise ERROR files: refusing to overwrite existing files (use
--force)` aborts `dotdrift apply`, and dotdrift had no `--force` to forward.
The refusal is `mise dotfiles apply` semantics (verified against mise
2026.9.1, `src/system/files.rs` `find_conflicts`): a **regular file** at a
symlink target "belongs to someone else"; only a symlink mise made is
re-pointable. Copy mode is unaffected (file-over-file is an ordinary
overwrite), as is `[bootstrap.files]`.

## Details

Real-world trigger in the dogfood profile: app-mutable configs (e.g.
`~/.dsh/settings.yaml`, rewritten by the app at runtime) declared as symlink
entries — the live regular file blocks convergence of the entire pipeline.

Fix: `dotdrift apply --force` threads through every dotfiles-apply path —
`DotfilesStep.Force` → `DotfilesApply(..., force)`, and the system edit
entries path in `systemFilesStep` (`DotfilesApplySudo`/`dotfilesApplyArgv`
gained the force parameter). Default stays off: refusal is the safety, and
onboard remains the deliberate takeover path. `[bootstrap.files]` needs no
flag (it overwrites file-over-file freely; node-type conflicts are
`replace = true` territory, not force).

## Acceptance Criteria

- [x] `dotdrift apply --force` passes `--force` to every `mise dotfiles apply` invocation (user scope, system edits user/elevated)
- [x] Default remains refusal (no `--force` in argv without the flag)
- [x] `go test ./...` and `go vet` green

## Out of Scope

- `[bootstrap.files]` node-type conflicts (`replace = true` — adopt when a profile needs it).
- A persistent "always force" config setting.

## Notes

Empirically verified against mise 2026.9.1: regular file at a symlink target
→ refusal without `--force`, replaced by the symlink with it.
