---
type: Issue
title: Pin working directory for all mise invocations
description: Issue 0039 pinned probe cwd only; apply-path mise runs (bootstrap, dotfiles apply, install, tasks) still load cwd configs additively, so a stray or broken mise.toml in the caller's cwd breaks apply.
tags: [issue, mise, bootstrap, cwd]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0041: Pin working directory for all mise invocations

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [mise, cwd]
- **Assignee**: agent
- **Related**: [0039](0039-tool-probe-cwd-immunity.md), [mise bootstrap alignment A2](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/mise.go`](../../internal/mise/mise.go)
- **Closing commits**: 58fafda

## Summary

0039 pinned the subprocess working directory to the account's home for
`Current` probes only. The apply path (`Bootstrap`, `DotfilesApply`,
`EnsureAndInstall`, `RunTask`) still inherits the caller's cwd, and mise loads
configs from the cwd upward additively (`--cd` does not suppress them) — a
stray or broken `mise.toml` in the caller's cwd fails every apply phase.

## Details

Extend the 0039 mechanism: every real mise subprocess runs with `cmd.Dir`
pinned (default: user home; overridable via the existing `Mise.ProbeDir`
field — consider renaming to a general `WorkDir` if the semantics widen).
The Run/RunContext test seams keep their shape and argv.

Watch for: hook tasks documented to run "from the profile root" — if any
generated task relies on the process cwd (e.g. `dir = "{{cwd}}"`), that
behavior must be preserved explicitly (generated configs should already
anchor paths; verify with the hooks tests).

## Acceptance Criteria

- [x] All real mise subprocess invocations run with a pinned working directory independent of the caller's cwd
- [x] Regression test: fake mise reports its pwd for each entry point (bootstrap, dotfiles apply, install, task)
- [x] Hooks still execute with their documented working directory semantics
- [x] `go test ./...` and `go vet` green

## Out of Scope

- Changing where dotdrift itself resolves the profile (`--profile` etc.).
