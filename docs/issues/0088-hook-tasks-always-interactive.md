---
type: Issue
title: Hook tasks are only generated interactive when the writing session had a TTY
description: GenerateHookTasks/GenerateApplyConfig gate mise's interactive = true on the writing session's stdin/handover state (0064-D4), so a hook task's interactivity — an intrinsic property — silently depends on how the config-writing run was attached.
tags: [bug, hooks, mise]
timestamp: 2026-09-21T23:00:00Z
---

# ISSUE 0088: Hook tasks are only generated interactive when the writing session had a TTY

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [hooks, mise, dogfooded]
- **Assignee**: none
- **Related**: [0064](0064-apply-session-tty-suspend-design.md) (D4 — the gate this supersedes), [0071](0071-surface-tty-handover-for-real-steps.md)
- **Related code**: [`internal/mise/mise.go`](../../internal/mise/mise.go), [`internal/service/session.go`](../../internal/service/session.go)
- **Closing commits**: 97e5961

## Summary

The generated apply config marked hook tasks `interactive = true` only
when the dotdrift run that wrote it had a terminal on stdin (or a
handover available). A hook's need for a terminal is intrinsic — it may
run `sudo` — but the key leaked away under piped/CI runs, and the config
outlives the run that wrote it: a later manual `mise run hooks-pre-0`
executes a task that silently lost its interactivity.

## Details

0064-D4 decided the config-write opt-in should key on handover
availability. That conflates two different questions:

1. **Is the task interactive?** An intrinsic property of the hook
   command. Belongs in the task unconditionally.
2. **Can this session put a real terminal under the child?** A
   session-level routing decision (handover vs piped) — legitimately
   keyed on `HandoverAvailable`.

mise has no `--interactive` CLI flag (the property is task-level TOML),
which is why the gate lived at config-write — but making the key
unconditional removes the need for the gate: mise 2026.9.10 (verified
empirically; dotdrift requires ≥ 2026.8.2) runs an `interactive = true`
task with piped stdout and `/dev/null` stdin exactly like a plain task —
exit 0, identical output, no warning. Under a pty it connects the TTY
(plus a harmless redactions hint).

## Acceptance Criteria

- [x] `GenerateHookTasks` marks every emitted task `interactive = true`
      unconditionally; the `interactive bool` parameter is gone
- [x] `GenerateApplyConfig` flows the same rule; the session's
      config-write no longer passes session state
- [x] An end-to-end apply test pins the shared mise.toml carrying the
      key with `StdinIsTerminal` false
- [x] Session-side routing is unchanged: handover-available sessions run
      hook children through `Handover`, others piped; the fail-loud rule
      (contract 13) stands
- [x] Comments claiming config-write keys on handover availability are
      corrected (step.go, deps.go, executil.go, session.go)

## Out of Scope

- The 0064 design doc itself (historical record; this issue supersedes
  D4's config-write half).
- Any change to `mise bootstrap`'s own TTY policy for non-task steps.

## Resolution

The parameter is deleted — always written, one call shape.
`HooksStep.Interactive` keeps its name and role (session routing only),
with its comment reworded. Verified red-first: the flipped tests failed
on the missing key with `StdinIsTerminal → false` before the change,
green after; full suite 20 packages ok.
