---
type: Issue
title: Writes dialogs refresh the nav — the message-flow sweep
description: Onboard and generate write into the profile but left the nav stale (the 0082 rule was manage-only); both construction sites now reload, restore provably does not, and every write path is driven end to end through Compositor.Update.
tags: [tui, dogfooded]
timestamp: 2026-09-21T22:40:00Z
---

# ISSUE 0085: Writes dialogs refresh the nav — the message-flow sweep

- **Type**: bug (stale nav) + test-infrastructure (the sweep)
- **Status**: done
- **Priority**: medium
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0082](0082-tui-override-module.md) (the reload rule, manage-only), [0084](0084-restore-dialog-plan-never-lands.md) (the delivery fix this builds on), [0083](0083-ponytail-audit-application.md) (found the 0084 class)
- **Related code**: [`internal/tui/keymap.go`](../../internal/tui/keymap.go), [`internal/tui/modals.go`](../../internal/tui/modals.go), [`internal/tui/dialog_test.go`](../../internal/tui/dialog_test.go)
- **Closing commits**: 60b5e8c

## Summary

0082 gave the manage dialogs a nav reload after a successful write, and
its round explicitly left the writes dialogs as a follow-up. Onboard and
generate write into the profile, so a new module never appeared in the
tree without a TUI restart. The reload hook now arms at both writes
construction sites — the `o` seam (`openOnboardInto`) and the `w` menu
(`openWritesDialog`) — while restore stays reload-free (it copies files
back to live targets and touches no profile dir).

## The sweep

0084's root cause was a class: a dialog returning a cmd whose message
must land through `Compositor.Update`, with tests that drove the dialog
directly and never pinned the delivery. The sweep closes the class by
driving every write path end to end through `Update`:

| Path | Compositor-level flow test | Pins |
|---|---|---|
| restore | `TestRestoreDialog_planLandsThroughCompositor` | plan delivery, real backup index, pin, run, no reload |
| onboard | `TestWritesDialogs_successfulWritesReloadNav` | confirm → run → report, reload = the nav read |
| generate | same test, second leg | shared-builder assembly, write, reload |
| manage (O key) | 0082's `TestOverride_oneKeyCreatesAndLands` | op, footer note, reload lands the workspace |
| failure path | `TestWritesDialogs_failedWriteKeepsNav` | a failed write re-reads nothing, error renders |

## Acceptance Criteria

- [x] A successful onboard or generate write re-runs the nav read (both construction sites)
- [x] Restore provably returns no reload
- [x] A failed write provably re-reads nothing
- [x] Every write path has at least one end-to-end `Update`-driven test

## Out of Scope

- Onboard/generate/restore auto-jumping the workspace to a newly created
  file (0082's `pendTab` handoff stays override-specific).
- The M14 shell (deleted surface) — the compositor is the only shell.
