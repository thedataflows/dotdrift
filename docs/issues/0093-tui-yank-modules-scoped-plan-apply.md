---
type: Issue
title: TUI nav yank — space/y marks modules, p/P plan and apply the yanked set only
description: With the left panel focused, space or y toggles a module's yanked mark; p (plan) and P (apply) then scope to the yanked modules instead of the whole profile. No yanks keeps the current default — all modules.
tags: [feature, tui, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0093: TUI nav yank — space/y marks modules, p/P plan and apply the yanked set only

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0064](0064-apply-session-service.md) (the `ApplyOpts.Modules` filter this rides), [0075](0075-tui-ergonomics-paging-selection-disclosure.md) (nav treatments)
- **Related code**: [`internal/tui/nav.go`](../../internal/tui/nav.go), [`internal/tui/keymap.go`](../../internal/tui/keymap.go), [`internal/tui/modals.go`](../../internal/tui/modals.go), [`internal/service/session.go`](../../internal/service/session.go)

## Summary

Dogfooded request: plan/apply in the TUI always covers the whole
profile. When you are iterating on two modules out of thirty you want a
vim-like mark set: yank the modules you are working on, then have `p`
and `P` act on exactly those.

## Details

- With the **nav pane focused**, `space` and `y` toggle the yanked mark
  on the selected row's module (a layer child row yanks its module).
  Yanked module rows carry a `✓` mark (a new `yankMark` theme entry —
  one style per visible concept).
- The yank set is session-only nav state; it survives layer reads and
  navigation.
- `p` (plan) and `P` (apply) pass the yanked module ids as
  `ApplyOpts.Modules` (the existing `LimitTo` filter the CLI's
  positional args already use). **Empty yank set = today's behavior
  (all modules)** — yanking is purely additive.
- The apply chain snapshots the set at `P` press and carries it through
  the gates (destructive confirm, elevation) into `Start`.
- The plan modal names the scope under its title when scoped.
- A nav reload prunes yanked ids that no longer name a module (delete
  module), so a stale yank can never surface as `unknown module(s)` from
  `LimitTo`.
- Help/footer come from the binding table, so the two bindings appear
  there like every other verb.

Out of scope: persisting the set across runs, yank-all/none chords, and
layer-granular yanks (the apply filter is module-granular, matching the
CLI).

## Acceptance Criteria

- [x] `space` and `y` on the nav pane toggle the selected module's yank
  mark; the row shows `✓` while yanked
- [x] `p` with yanks previews exactly the yanked modules; the plan modal
  names the scope
- [x] `P` with yanks starts the apply session with `Modules` set to the
  yanked ids (snapshot at press, carried through the gates)
- [x] With no yanks, `p`/`P` pass a nil module filter (all modules —
  unchanged)
- [x] A nav reload drops yanked ids whose module disappeared
- [x] `go test ./...` passes; docs updated (tui.md keymap, log.md)

## Notes

Fixed in the closing commit for 0091/0092/0093 (TDD: `yank_test.go`
red first — toggle+mark, scoped plan, scoped apply, reload prune).
