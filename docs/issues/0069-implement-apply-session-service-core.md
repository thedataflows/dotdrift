---
type: Issue
title: Implement apply-session service core
description: Build the 0064-designed apply session in internal/service — run handle, event vocabulary, output policies, cancel, handover seam, absorbed orchestration.
tags: [implementation, apply, api, service]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0069: Implement apply-session service core

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0064 apply session design](0064-apply-session-tty-suspend-design.md), [0061 service-layer architecture](0061-service-layer-architecture-cli-migration.md), [contract invariants 2, 11](../product/contract.md)
- **Related code**: `internal/service/` (new), [`internal/apply/`](../../internal/apply/), [`internal/mise/mise.go`](../../internal/mise/), [`cmd/apply.go`](../../cmd/apply.go)
- **Blocked by**: [0064 Apply session & TTY suspend design](0064-apply-session-tty-suspend-design.md)
- **Closing commits**: d40e498

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

## Resolution

Implemented in d40e498 (TDD: the 0064 event-sequence spec written as
acceptance tests first, red at the package seam, green per cycle). The
session is live in `internal/service` with all 17 acceptance scenarios
passing, `go vet` clean and `-race` clean.

**Landed**

- `ApplyArea.Start` run handle: resolve → `state.TryLock` (typed
  `AlreadyRunningError`; reads stay lock-free) → classify → run
  goroutine emitting `PlanResolved` (cursor + `CursorEffective`) →
  [`BackupTaken` per receiving module dir] → per-step
  `StepStarted`/`StepOutput`/`StepFinished`/`StepFailed` →
  `SessionEnded`; state file removed on completion; `Wait` returns
  `SessionResult` for every decision, erroring only
  `*SessionCancelledError` (D1/D3/D6/D7).
- D2 output policies: `Output` attached = children stream to it fd-direct
  (terminal writers keep child color, exactly `StreamLive` semantics); nil
  = new `mise.Mise.ForceStream` streams to a line-buffered collector
  without the verbose echo; trailing partial lines flush at step end.
- D5 cancel: `mise`'s streaming `runOp` branch gained the `Setpgid` +
  process-group SIGKILL cancel `runContextEnv` already had (the gap
  research 0059 §5 named), shared via `setGroupKill`; cancel mid-step
  reports `Cancelled` with the interrupted step, cursor untouched.
- D9 handover: the session derives a **session-ctx cmd twin** from the
  step's command spec — `Setpgid` on, `Cancel` = group SIGKILL riding
  exec's own ctx watcher (no process-handle race with the consumer's
  `Start`), stdio left nil for the consumer to wire — and hands it to
  `opts.Handover`. A refusal fails the step as a resumable `*StepError`.
  Reviewed by two subagents: the first watchdog draft raced
  `cmd.Process`; the stdlib-machinery shape replaced it.
- Absorbed orchestration: step types + `buildSteps` + `backupCopyTargets`
  (→ events, not printed lines) + `writeBootstrapConfig` + config-path
  layout moved from `cmd/apply.go` behind `ApplyDeps` seams; section
  vocabulary single-sourced as `service.SectionNames` /
  `ResolveSections` (cmd aliases it). **cmd/apply.go itself is untouched**
  — its step code stays until 0070 deletes it.
- Review-driven hardening beyond the original tests: cursor rolls back
  when a step's persist fails (it never names an unpersisted step,
  contract 2); a genuine step failure stays `Failed` even when the
  consumer's ctx dies mid-failure (the `StepFailed` event is not
  rewritten to `Cancelled`); cancel is proven to kill a child
  **mid-handover**; `configPaths` struct replaces the magic-string path
  map; sections live in their own file.

**Realization notes** (sanctioned deviations, per scope notes above):
`ApplyOpts.HandoverAvailable *bool` keys the interactive-hook opt-in
(zero-value = deps' stdin reality); `PlanResolved` carries `Profile` +
`Facts` for the CLI's `printPlan`; hook `Sub` stays nil (no event lies
about granularity that does not exist — 0071); real steps implement no
handover yet (fakes only — 0071); cmd keeps a documented duplicated
orchestration until 0070 (sections.go included — review flagged it,
0070's scope now names it).
