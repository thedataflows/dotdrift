---
type: Issue
title: TUI nav quick filter — / filters the module list in place, no modal
description: The nav pane's slash key filters the module list in place — typed characters narrow rows live through the palette's fuzzy matcher, the query shows at the pane bottom, enter keeps the filter, esc removes it; the workspace pane's slash keeps the palette.
tags: [issue, tui, ergonomics, navigation]
timestamp: 2026-09-21T14:32:21Z
---

# ISSUE 0081: TUI nav quick filter — in-place module filtering

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ergonomics, navigation]
- **Assignee**: none
- **Related**: [0078](0078-tui-enter-fallback-and-footer-hints.md), [0079](0079-tui-undo-marks-onboard-cycle.md)
- **Related code**: [`internal/tui/nav.go`](../../internal/tui/nav.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go), [`internal/tui/keymap.go`](../../internal/tui/keymap.go)
- **Closing commits**: 13187f8

## Summary

The nav pane's `/` opened the palette — a centered modal — when the only
job at hand was narrowing the module list. User ask: filter the left
modules list in place, show what is being typed at the bottom, and
remove the filter on esc.

## Details

One task, TDD-first (**T-tui-navfilter**):

- **The `/` binding splits per pane** in the binding table: nav →
  `filter modules` (starts the in-place filter), work → `palette`
  (unchanged). The footer's short help resolves `/` from the table per
  focused pane, so the hint cannot drift from behavior; the `?` help
  follows the same table.
- **The filter is nav state, not a modal** (`navModel.query`,
  `navModel.filtering`): `rows()` drops modules that do not match the
  query, matching through `fuzzy.Find` — the palette's own matcher, so
  both surfaces agree on what "matches" means — and keeps declaration
  order. Layer children of matched modules render per expansion, and
  the dirty markers, reasons, and mouse hit-testing all keep working
  because they ride the same `rows()`.
- **The typing mode owns keys** (the compositor's base-key path hands
  `KeyPressMsg` to `navFilterKey` while it is active): printable
  characters extend the query, backspace trims it, `j`/`k`/arrows move
  within the matches (the workspace follows live through the usual
  `syncWorkspace` seam), enter leaves the mode with the filter applied,
  esc removes the filter and leaves the mode. Any other message (mouse)
  falls through to the base handlers; a click commits the mode and
  keeps the filter.
- **The query shows at the pane bottom**: while typing, the cursor-bar
  line `/ demo▏`; after enter, a named applied filter
  `/ demo · esc clears`. The line reserves the pane's last row (the
  scroll window accounts for it), so rows never render underneath it.
  An empty result says `no modules match «query»` in the dim style.
- **The cursor never loses its module**: after each query change,
  `refilter` re-finds the module the cursor was on; the first match
  takes the cursor only when it no longer matches. The base esc rule
  gained one clause before its focus-to-nav rule: an applied filter
  clears first.

Deliberately rejected: replacing the palette (it still owns the
actions/fields/recents scopes and `ctrl+n` module creation from the
work pane), and re-ranking matches by score (a filter keeps tree order;
ranking is the palette's job). `pgup`/`pgdown` inside the typing mode
are not bound — enter first, then page.

## Acceptance

- `/` on the nav pane opens no modal, sets the filtering mode, and
  typing narrows `rows()` to the matching modules with the cursor on
  the match (`TestFilter_slashFiltersListInPlace`).
- The frame shows `/ de▏` while typing and `/ de · esc clears` after
  enter, with the filter still applied; esc — in the mode or after it —
  restores the full list and removes the line
  (`TestFilter_queryShownAtPaneBottom`, `TestFilter_escRemovesFilter`).
- A no-match query renders `no modules match «zzz»` and `j` does not
  panic on the empty list (`TestFilter_noMatchesNamesTheQuery`).
- `j` moves within the matches while typing, enter ends the mode, and a
  second enter opens the selection in the workspace
  (`TestFilter_arrowsMoveWhileTyping`).
- The cursor keeps its module while it still matches
  (`TestFilter_cursorStaysOnMatch`).
- `/` on the workspace pane still opens the palette, and the footer's
  `/` hint names the focused pane's verb — `filter modules` on nav,
  `palette` on work (`TestFilter_workSlashStillOpensPalette`,
  `TestFilter_footerHintFollowsPane`).
- A mouse click while typing commits the mode and keeps the filter
  (`TestFilter_clickSelectsAndExitsMode`).

## Verification

Fresh runs on the final tree: `go test ./... -count=1` — 20 packages
ok, 0 FAIL; `go vet ./...` exit 0; gofmt clean on touched files;
golangci-lint `run ./internal/tui/...` — 0 issues (read-only default
cache warnings are environmental). Golden sweep: 26 frames
regenerated, and the full diff audited as tokens — 22 nav-focused
frames change only the footer verb (`/ palette` →
`/ filter modules`); the 4 palette frames change only the dimmed
base's footer line, because their test helper now opens the palette
from the work pane (the palette box itself is unchanged). TDD
red→green: the 9 new tests failed first on the missing
`navModel` fields; the RED run caught one production bug — the
filter-mode branch swallowed mouse messages with an unconditional
return, so clicks went dead while typing. Test-site migrations:
`paletteShell` and two direct callers focus the work pane before `/`
(the palette's pane moved), `TestHelp_contextual` asserts the nav
help's new verb, and `TestKeys_noOrphanedBindings` pins `/` as two
per-pane rows. Ponytail audit: the filter is two fields and four small
functions on the nav model, matching reuses the palette's matcher, and
no new dependencies or abstractions — the alternative (reusing the
palette model in-place) would have dragged actions/fields/recents
state into the tree pane.
