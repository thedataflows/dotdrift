---
type: Issue
title: Migrate cmd/apply onto the apply session
description: Rewire ApplyCmd.Run onto internal/service's session — flags to ApplyOpts, event rendering — and delete the duplicated orchestration.
tags: [implementation, apply, cli, migration]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0070: Migrate cmd/apply onto the apply session

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: cri
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0069 apply-session service core](0069-implement-apply-session-service-core.md), [0061-D7 migration order](0061-service-layer-architecture-cli-migration.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), `internal/service/`
- **Blocked by**: [0069 Implement apply-session service core](0069-implement-apply-session-service-core.md)
- **Closing commits**: 7c7bb1e

## Question

Rewire `ApplyCmd.Run` as the 0061-D7 adapter: translate flags →
`ApplyOpts` (`Sections` from the section flags via `resolveSections`,
`Output` = the process streams for passthrough, `HandoverAvailable` =
its real stdin), drain the session's events, and render — `printPlan`
on `PlanResolved` (byte-identical), backup lines on `BackupTaken`,
`--diff` as a reads call before `Start` — then **delete** the duplicated
orchestration this migration orphans: the step types, `buildSteps`,
`backupCopyTargets`, `writeBootstrapConfig`, config-path wiring copied
into `internal/service` by 0069, **and the section-flag twins in
`cmd/sections.go`** (`sectionSet`, `newSectionSet`, `resolveSections`,
`isSectionName` — the flag layer reduces to kong parsing plus
`service.ResolveSections`). Nothing forks: after this ticket the
session is the only apply orchestration in the tree. The existing cmd
apply tests are the byte-identical gate; their seam setup moves from
package-level vars to `ApplyDeps` injection.

## Acceptance Criteria

- [ ] `cmd/apply.go` contains only flag translation, session drain, and
      event rendering; the duplicated step code is deleted.
- [ ] All existing cmd apply tests pass unchanged in their assertions
      (golden output parity), with seams injected via `ApplyDeps`.
- [ ] `--diff`, `--backup`, section flags, `DOTDRIFT_NO_HOOKS`,
      `--verbose` behave exactly as today.

## Resolution

Implemented in 7c7bb1e. `ApplyCmd.Run` is now the 0061-D7 adapter
(~290 lines in cmd/apply.go, down from 737; cmd/sections.go from 128 to
~47): deps-driven display reads → flags → `ApplyOpts` → `Start` → event
drain. Every acceptance criterion holds — all deletions confirmed
absent from cmd (step types, `buildSteps`, `backupCopyTargets`,
`writeBootstrapConfig`, config-path wiring, the four section twins),
cmd tests keep their assertions with seams stubbed via `ApplyDeps`
injection, and `--diff`/`--backup`/section flags/`DOTDRIFT_NO_HOOKS`/
`--verbose` behave as before. Two-axis review (standards + spec) plus a
whole-repo ponytail audit ran before commit; their actionable findings
drove four fixes below.

Decisions taken while implementing (all within the accepted designs):

- **Display reads pre-Start.** `printPlan` and `--diff` render from an
  adapter-side reads pass (`displayReads`, deps-driven), not from the
  `PlanResolved` event: the run goroutine starts writing as soon as
  `Start` returns, so rendering plan/diff from events would race the
  apply (and flip the plan→diff output order). `PlanResolved` only
  opens the stream for the CLI. Cost: detect/load/resolve run twice per
  apply (reads are pure; the session re-runs them in its own goroutine).
- **`--verbose` stays flag-layer** (0064-D8): the adapter wraps the
  injected deps with capture-then-wrap closures (`NewMise` sets
  `Verbose`, `PackagesFor`/`NewSmbRunner` `SetVerbose(true)`); `ApplyOpts`
  carries no Verbose field.
- **`service.ResolveSections` gains unknown-name rejection** with the
  deleted `newSectionSet`'s exact text — the override path's
  fail-loud contract moved with the logic; red-tested service-first.
- **Byte-parity fixes the reviews caught** (both red→green):
  - `BackupTaken.Count` (written count) — `backup.Run` skips
    nonexistent destinations; the old line printed the written count,
    `len(Files)` (the request) overstated on fresh hosts. The CLI line
    prints `Count`.
  - Passthrough mode leaves the child's stderr on the process stderr
    (`spec.m.Err = nil`; mise's `writers()` defaults it) — merging into
    `Output` moved mise warnings and the `--verbose` echo onto stdout,
    unlike the old nil-writer wiring. The stdout side carries
    `opts.Output` exactly as D2 pins.
- **Orphaned blocking lock deleted** (ponytail audit finding):
  `state.FileStore.Lock` had zero production callers once apply moved
  to `TryLock`; the blocking API, `TestFileStore_lockBlocksUntilUnlock`,
  and `cmd/apply_lock_test.go` (which pinned the removed queueing
  contract) are gone. `TestApply_alreadyRunningErrors` pins the new
  refusal surfacing.
- **Test placement**: the white-box `systemFilesStep`/writability tests
  moved to `internal/service` (the step types' new home) and
  `TestPathUserWritable` to `internal/executil`; the ResolveSections
  truth table is pinned once in the service, with cmd keeping only the
  flag-layer concerns (kong parsing, presence detection, env
  kill-switch, override path).

Documented behavior deltas (deliberate): concurrent applies now refuse
with `AlreadyRunningError` instead of queueing (0061-D8); plan/diff
output precedes a lock refusal (reads happen before `Start`). Deferred,
not forgotten: unifying `displayReads` with the package-var read
preamble (`loadAndResolve` trio) happens when plan/status migrate onto
the service — the vars stay for those commands today.
