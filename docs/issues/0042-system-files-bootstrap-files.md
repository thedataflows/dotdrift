---
type: Issue
title: System files via bootstrap.files instead of dotfiles+sudo
description: Move system-scope dotfile convergence to mise's native [bootstrap.files] (privilege batching, atomic writes, owner/group/mode) and delete dotdrift's hand-rolled sudo retry machinery.
tags: [issue, mise, bootstrap, dotfiles, scope]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0042: System files via [bootstrap.files], delete hand-rolled sudo

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [mise, bootstrap, deletion]
- **Assignee**: agent
- **Related**: [mise bootstrap alignment A3](../product/mise-bootstrap-alignment.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), [`internal/mise/mise.go`](../../internal/mise/mise.go), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: 0bed6ed

## Summary

`systemFilesStep` translates system-scope dotfiles into `[dotfiles]` entries
and applies them via `mise dotfiles apply`, retrying the whole run elevated
(`DotfilesApplySudo` + `dotfilesApplyArgv`) on permission errors. mise's
`[bootstrap.files]` does this natively and better: try-as-user then retry the
remaining changes in one privileged batch, atomic temp+rename writes, and
`owner`/`group`/`mode` convergence.

## Details

Alignment finding A3. The step should emit `[bootstrap.files]` (it already
has `ResolveBootstrapFiles`/`GenerateBootstrapFiles` — currently used only
for translation input) and run `mise bootstrap --only files`, letting mise
elevate. Templates keep `template = true`. The symlink→copy safety
translation stays (bootstrap.files has no symlink mode — it manages content).

Delete when done: `DotfilesApplySudo`, `dotfilesApplyArgv`, the sudo
try/retry path in `systemFilesStep`, and their tests; keep
`ResolveBootstrapFiles` (symlink-each expansion). Mount destination
directories already go through `[bootstrap.directories]` — verify they ride
the same `bootstrap` invocation or remain their own step.

Status (`dotdrift status`) keeps its own drift probes — converging status
onto `mise bootstrap files status --json` is a standing consideration, not
this issue.

## Acceptance Criteria

- [x] System-scope whole-file dotfiles converge via `[bootstrap.files]` + `mise bootstrap --only files`
- [x] Edit entries still pass through `[dotfiles]` as today (bootstrap.files has no edit concept)
- [x] ~~`DotfilesApplySudo`/`dotfilesApplyArgv` and the sudo retry path are deleted~~ **amended**: the whole-file sudo retry path, the writability pre-flight over whole-file targets, and `ensureDir` are deleted; `DotfilesApplySudo`/`dotfilesApplyArgv` survive — system EDIT entries have no bootstrap.files equivalent and still elevate through them (contract #18). `DotfilesSystemStep` and `SystemDotfileEntries` (dead after the move) were deleted instead
- [x] Elevation failures surface mise's own actionable error (verified empirically against mise 2026.9.1: `sudo requires a password but no TTY is available. Run manually: sudo /usr/bin/mise --no-config ... bootstrap __apply-system-plan`)
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Emitting `owner`/`group`/`mode` from module.toml (no schema yet — adopt when needed).
- `replace`, `state = "absent"`, `notify` on bootstrap.files entries.
- Status-engine convergence onto mise bootstrap status.

## Notes

mise escalates interactively (sudo prompt on a terminal; exact-command error
when non-interactive without passwordless sudo) — that matches dotdrift's
fail-loud posture better than the current whole-run sudo retry.

## Resolution notes

- The August revert (dfb5745: "hidden sudo helpers needing a TTY mise could
  not reach") no longer applies: mise's `src/system/sudo.rs` policy (TTY →
  interactive prompt; no TTY → `sudo -n` fail-fast with the exact command)
  plus dotdrift's `opStdin` wiring cover both cases. Verified empirically
  against the installed mise 2026.9.1 (user-writable target, EACCES target,
  template rendering, `--only files` covering `[bootstrap.directories]`).
- Mount destination directories moved from the `ensureDir` try/retry seam to
  `[bootstrap.directories]` in the same `--only files` call (still
  section-gated: no mounts section ⇒ no directories in the config).
