---
type: Issue
title: Implement apply-session service core
description: Build the 0064-designed apply session in internal/service — run handle, event vocabulary, output policies, cancel, handover seam, absorbed orchestration.
tags: [implementation, apply, api, service]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0069: Implement apply-session service core

- **Type**: task
- **Status**: in-progress
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0064 apply session design](0064-apply-session-tty-suspend-design.md), [0061 service-layer architecture](0061-service-layer-architecture-cli-migration.md), [contract invariants 2, 11](../product/contract.md)
- **Related code**: `internal/service/` (new), [`internal/apply/`](../../internal/apply/), [`internal/mise/mise.go`](../../internal/mise/), [`cmd/apply.go`](../../cmd/apply.go)
- **Blocked by**: [0064 Apply session & TTY suspend design](0064-apply-session-tty-suspend-design.md)
- **Closing commits**: none

## Question

The design is settled (0064 D1–D10). Build it: the `internal/service`
apply session as a run handle — `Start` (resolve → `TryLock` → classify →
`PlanResolved`) on one goroutine streaming the closed event vocabulary,
`Preview()`, `Cancel()` (immediate process-group kill, cursor names the
last completed step), `Wait() (*SessionResult, error)` — with
`cmd/apply.go`'s orchestration absorbed (sections → buildSteps, D8a
snapshot, backup pre-step, cursor ownership, config paths) and the D2
output policy (`Output` attached = passthrough, absent = line-buffered
`StepOutput` events). Process: ATDD against the 0064 event-sequence
spec; seams are the designed public API.

## Scope notes (realizations, not design changes)

- **CLI untouched this slice.** `cmd/apply.go` keeps its own copy of the
  step code until [0070](0070-migrate-cmd-apply-onto-apply-session.md)
  rewires it onto the session and deletes the duplicate — the 0061-D7
  migration order (apply session last) with the existing cmd tests as
  the byte-identical gate.
- `ApplyOpts` gains `HandoverAvailable *bool` (D4's absorption row:
  "CLI passes its own reality, TUI passes true"). Nil = `deps`
  `stdinIsTerminal()` (today's behavior, zero-value safe); non-nil
  overrides — this is what keys the `interactive = true` hook opt-in at
  config-write.
- `PlanResolved` carries `Profile` and `Facts` alongside `Plan`: the CLI
  adapter's `printPlan(out, plan, p, f, deps)` needs all three, and D2
  routes plan rendering through the adapter.
- Hook sub-steps (`StepStarted.Sub`, D3) land with
  [0071](0071-surface-tty-handover-for-real-steps.md): today's
  `HooksStep` runs its commands inside one `Run`, so per-command
  boundaries are not observable yet; `Sub` stays nil and no event lies.
- `mise`'s streaming `runOp` branch gains the `Setpgid` + process-group
  SIGKILL cancel that `runContextEnv` already has (0064-D5's named gap,
  research 0059 §5), plus a `ForceStream` field so event mode streams to
  the collector without the `--verbose` echo.

## Acceptance Criteria

- [ ] Session lifecycle matches the 0064 event sequence: `PlanResolved`
      (cursor + effectiveness) → [`BackupTaken`] → per-step
      `StepStarted`/(`StepOutput`)/`StepFinished`/`StepFailed` →
      `SessionEnded`; state file removed on completion (contract 2).
- [ ] `AlreadyRunningError` from `Start` when the sidecar lock is held;
      reads stay lock-free (contract 11).
- [ ] Cancel mid-step kills immediately, reports `Cancelled` with the
      interrupted step, cursor untouched; cancel after completion is a
      no-op.
- [ ] Output policy: attached writer = no `StepOutput` events; absent =
      line-buffered chunk events; event mode forces streaming without
      the verbose echo.
- [ ] Handover seam: session-enforced `Setpgid` + nil stdio on the
      consumer-run cmd; refused handover fails the step as a resumable
      `*StepError`.
- [ ] `Preview()` reports per-step TTY needs + reasons, stable after
      `Start`.
- [ ] Resume + stale-cursor rules unchanged (contract 2); sections
      filter works.
- [ ] `go test ./...` green; `mise` group-kill + force-stream covered.
