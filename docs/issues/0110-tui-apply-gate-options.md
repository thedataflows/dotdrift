---
type: Issue
title: TUI apply gate options (force, backup, sections)
description: An options gate on every P, before the preview — force, backup, and one toggle row per plan section.
tags: [issue, tui, views]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0110: TUI apply gate options

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, views]
- **Assignee**: agent
- **Related**: [TUI views design spec](../superpowers/specs/2026-10-18-tui-views-design.md), slice 4
- **Related code**: [`internal/tui/modals.go`](../../internal/tui/modals.go)
- **Closing commits**: 9e8ee27

## Summary

`P` opens an options gate before the preview: force (off), backup
(off), and one toggle row per plan section of `service.SectionNames`
(all on). Confirming snapshots the choices into
`service.ApplyOpts{Force, Backup, Sections}`; declining runs nothing.

## Details

Slice 4 of the TUI views design. The gate sits in front of the existing
chain: confirm (`y`/`enter`) runs exactly one `Preview` with the
snapshot, then the destructive confirm and the elevation gate run as
before, and the same snapshot rides into `Start`. Decline (`esc`) runs
nothing — no preview, no run. The modal is built fresh on every press,
so the choices reset. An all-on section selection snapshots to nil
(`ApplyOpts`' "nil = all" rule): a default confirm is today's behavior.

With backup on, the run takes backups and the `BackupTaken` event —
already rendered by the apply detail modal but unreachable before,
because the TUI never set `Backup` — now lands in the detail view.

## Acceptance Criteria

- [x] The gate opens on every `P`, before any preview; force and backup
  default off, every section on
- [x] `y`/`enter` snapshots the choices; exactly one `Preview` runs with
  them; the same `Force`/`Backup`/`Sections` ride into `Start`
- [x] A deselected section shrinks the preview's step list
- [x] `esc` runs nothing; the footer reports the decline
- [x] Choices reset on the next `P`
- [x] With backup on, the `BackupTaken` line renders in the apply detail
- [x] Keys only through the modal's own update + the compositor pop
  rule; the gate's hints live in the contextual `?` help

## Out of Scope

- Persisting the choices across presses
- A "no sections selected" guard (the service's resolve error surfaces
  in the footer, loud)

## Notes

Pinning tests: `TestApplyGate_optionsBeforePreview`,
`TestApplyGate_declineRunsNothing`, `TestApplyGate_resetsEachPress`,
`TestApplyGate_backupEventRenders`. Every pre-existing `P`-driving test
now passes through the gate via the `applyGateY` helper.
