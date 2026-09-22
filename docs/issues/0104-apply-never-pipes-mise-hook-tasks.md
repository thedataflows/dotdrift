---
type: Issue
title: apply still pipes mise hook tasks when stdin is not a terminal
description: HooksStep routes tasks through the piped runner whenever StdinIsTerminal is false, silently stripping the terminal that the tasks' own interactive = true (0088) promises; mise tasks must run through the handover seam everywhere.
tags: [bug, hooks, mise, apply]
timestamp: 2026-09-23T00:00:00Z
---

# ISSUE 0104: apply still pipes mise hook tasks when stdin is not a terminal

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [hooks, mise, apply]
- **Assignee**: none
- **Related**: [0088](0088-hook-tasks-always-interactive.md) (the config half this completes), [0071](0071-surface-tty-handover-for-real-steps.md), [0064](0064-apply-session-tty-suspend-design.md) (D4 — the routing gate this deletes)
- **Related code**: [`internal/mise/step.go`](../../internal/mise/step.go), [`internal/service/session.go`](../../internal/service/session.go), [`cmd/apply.go`](../../cmd/apply.go)
- **Closing commits**: f0cc560

## Summary

0088 made every generated hook task declare mise's
`interactive = true` unconditionally. The execution side still gates:
`HooksStep.Interactive` keys on `StdinIsTerminal()` for the CLI, so
`dotdrift apply` run without a terminal on stdin (piped, redirected,
ssh without a tty, scripts, CI) executes hook tasks through the piped
runner — captured output, null stdin — and the interactivity the config
promises is stripped by dotdrift itself. mise tasks must be interactive
everywhere: apply never pipes them.

## Details

The gate is a session-attachment property — exactly the class of bug
0088 removed from the config side; it survived on the execution side.
`runOp`'s piped path captures stdout/stderr when the writers are not
terminals and feeds the child the null device as stdin (`opStdin`),
so a hook needing a prompt (sudo password, any question) cannot work,
even when a terminal exists on the other fds or on the controlling tty.

The fix is deletion: `HooksStep` always routes tasks through the
handover seam (`RunTaskCmd` + `Handover`), the one honest path — the
consumer wires the child to its real stdio (CLI fd passthrough, TUI
terminal suspension). When the caller truly has no terminal, the child
sees pipes/devnull and mise itself degenerates the task to plain
execution (verified against 2026.9.10 in 0088: exit 0, identical
output) — same outcome as today's piped run, but dotdrift never
demotes, and output streams instead of being swallowed. A consumer
that cannot hand over at all (`Handover == nil`) fails loud, per
contract 13.

`ApplyDeps.StdinIsTerminal` and `ApplyOpts.HandoverAvailable` lose
their only consumer and die with the gate. `ExecMise.RunTask` — the
piped task runner — becomes dead and is deleted.

## Acceptance Criteria

- [x] `HooksStep` has no `Interactive` field; every task runs through
      the injected `Handover` as a spec-built `mise run` child
- [x] Without a `Handover` callback the step fails loud naming the
      handover contract; no task runs
- [x] An end-to-end session test pins hooks reaching the handover with
      no terminal in the test process and no override
- [x] The session-level `StdinIsTerminal`/`HandoverAvailable` knobs are
      gone (`ApplyDeps`/`ApplyOpts`), including the TUI's override
- [x] `ExecMise.RunTask` is deleted with its direct tests; remaining
      runner tests cover the surviving entry points

## Out of Scope

- The 0064 design doc and historical task sheets (records, not specs).
- `mise bootstrap` / `mise dotfiles apply` interactivity (commands, not
  tasks; they keep their own `--yes`/prompt policy).
- Contract 13's elevation fail-loud rule (unchanged).

## Notes

The TUI already routed hooks through its handover unconditionally, so
TUI behavior is unchanged by the deletion. The CLI's tty run already
handovered; only non-tty runs change (streamed instead of captured —
strictly more visible, since `RunTask` discarded the captured output
even on failure).

## Resolution

The gate is deleted — hooks always handover, routing knobs removed,
`RunTask` deleted (net −66 lines). Verified red-first: the rewritten
step tests failed on `RequiresTTY` being empty and the missing loud
error, and the session test failed on `NeedsTTY` false, all on the
piped path before the change; green after. Full suite all packages ok,
`go vet` clean, golangci-lint 0 issues on touched packages, gofmt
clean.
