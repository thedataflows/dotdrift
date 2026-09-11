---
type: Issue
title: Surface TTY handover for real steps
description: Make mise-runner steps emit the child commands that need the terminal (sudo elevation, interactive hooks) through the session's Handover seam, with hook sub-steps.
tags: [implementation, apply, tui, mise]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0071: Surface TTY handover for real steps

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: dev
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0069 apply-session service core](0069-implement-apply-session-service-core.md), [0064 D3/D4/D9](0064-apply-session-tty-suspend-design.md), [contract invariants 12, 13](../product/contract.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), `internal/service/`
- **Blocked by**: [0069 Implement apply-session service core](0069-implement-apply-session-service-core.md)
- **Closing commits**: bdd2a21 (hook sub-step events), 3b882ae (interactive hooks through Handover), d032672 (sudo edits through Handover), e3d0d31 (step-seam pins), d453d12 (shared SetGroupKill), 4a088f2 (doc repairs), 30f86d7 (review fixes)

## Question

0069 ships the `Handover` seam and tests it with fake steps; no real
step declares a terminal need yet. (Realization note: the session hands
consumers a session-ctx cmd **twin** of the step's spec — `Setpgid` +
group-SIGKILL via exec's own watcher, stdio nil — so 0071's steps only
supply `Path`/`Args`/`Env`/`Dir` and call the injected callback.) This
ticket makes the honest
classification real: the mise-runner steps surface the child commands
that truly need the terminal — sudo-elevated dotfiles/system applies
(`DotfilesApplySudo` path), interactive `mise run` hook tasks — through
the session's handover wrapper (session-enforced `Setpgid` + nil stdio,
consumer-run), so `Preview()` reports true `NeedsTTY` + reasons and a
TUI can `tea.ExecProcess` them. Includes D3's hook sub-steps: split
`HooksStep` so each hook command's start/fail is observable
(`StepStarted.Sub{Index, Total, Command}`), and D4's classification
grounded in the actual per-step elevation/interactivity decisions
(euid, writability probes, `interactive = true` declarations) instead
of the placeholder interface.

## Acceptance Criteria

- [x] Steps that elevate or declare interactive hooks implement the
      handover interface; their children run through `Handover` with
      session-enforced process-group semantics (contract 12/13 hold).
- [x] `Preview()` reasons name the real cause (sudo path, interactive
      hook) per step.
- [x] Hook steps emit per-command sub-step events.
- [x] CLI passthrough mode stays byte-identical (handover wiring equals
      today's direct-fd path).

## Resolution

Implemented 2026-09-12 in five slices, TDD red-green at the service and
step seams.

- **Hook sub-step events** (bdd2a21): apply.Observer gained
  HookStarted/HookFailed (carrying apply.SubStep{Index, Total, Command});
  the pipeline injects the observer into steps implementing
  observerSetter; HooksStep fires per-command boundaries; the session
  surfaces them as Sub-carrying StepStarted/StepFailed events. This
  extends 0064-D3's sketch, which put Sub on StepStarted only - a
  failing hook command must be observable before the step-level
  failure, so StepFailed gained Sub too (required hooks: sub failure
  then step failure; optional: sub failure, sequence continues).
- **Interactive hooks through Handover** (3b882ae): HooksStep gains
  Interactive + Handover (implements HandoverStep); with the opt-in each
  task runs as a spec-built "mise run" child through the consumer's
  terminal. mise.newOpCmd extracted runOp's cmd construction (env, work
  dir, verbose echo, nil stdio) so streaming and handover share one
  builder - the byte-parity anchor.
- **Sudo edits through Handover** (d032672): systemFilesStep implements
  HandoverStep off one needsSudo predicate shared by RequiresTTY and Run
  (classification cannot drift from behavior). The elevated child is
  "sudo -E mise dotfiles apply" with the trust plumbing intact; mise's
  streaming DotfilesApplySudo runner is deleted with its dead test
  cases (converted to DotfilesApplySudoCmd).
- **Fail-loud symmetry** (review, 30f86d7): Interactive with no handover
  callback errors like the system-edits path (contract 13) -
  classifying NeedsTTY and then piping an interactive command would lie.
- **Shared group-kill** (audit, d453d12): executil.SetGroupKill is the
  one Setpgid + group-SIGKILL policy (mise streaming + handover twins).

Byte-parity notes (AC4): cmd suites stay untouched-green; cmd test dep
stubs pin StdinIsTerminal=false because go test's stdin (/dev/null) is a
char device and silently activated the interactive path - the
interactive path is covered at the service seam; handoverToTerminal
became a package var so tests stand in for the terminal exec.
Documented edge deltas vs the pre-change path: the handover twin keeps
WaitDelay=5s (cancel-path tail truncation only - D5's reap-the-tree
intent), and the stdin-TTY + piped-stdout mixed case now fd-directs
through handover (byte destination identical; the child may emit color
the old captured path withheld - desirable). Preview freezing at Start
is D4-by-design; the shared predicates keep it honest.

Preview() reasons name the real causes ("elevated system edits: edit
targets are not user-writable (sudo)", "interactive hook commands run
on your terminal"). Reviews: standards (3 hard: status spelling, log
entry, gofmt - all fixed) + spec (no blocking findings; the fail-loud
inconsistency was flagged by both axes and fixed) + whole-repo ponytail
audit (shared group-kill applied; stale doc refs repaired).
go test ./..., go vet, race on touched packages: green, 19 packages.
