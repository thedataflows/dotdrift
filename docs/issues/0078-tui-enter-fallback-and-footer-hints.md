---
type: Issue
title: TUI discoverability round — enter falls back to the add form, the footer hints name the primary verbs
description: Enter on a row that owns no field opens the same add form `a` opens, and the footer hints render enter/a/d/ctrl+s instead of the first three scroll bindings.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-20T00:00:00Z
---

# ISSUE 0078: TUI discoverability round — enter falls back to the add form, the footer hints name the primary verbs

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [tui, ergonomics]
- **Assignee**: none
- **Related**: [0077](0077-tui-add-forms.md), [0076](0076-tui-choice-editors-and-location.md), [0075](0075-tui-ergonomics-paging-selection-disclosure.md)
- **Related code**: [`internal/tui/editing.go`](../../internal/tui/editing.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go)
- **Closing commits**: 7191f11, 5b5cced

## Summary

The 0077 add forms were reachable only through `a`, and `a` was itself
invisible: the footer's short help took the first three work-pane rows
of the binding table — all scrolling (`j scroll down • k scroll up •
pgup page up`) — so the two most important keys, `enter` and `a`,
never rendered. And enter on an empty section's header or a structural
container did nothing: `startEdit` refused those rows silently, while
the user's context said "grow this section".

## Details

Two tasks, TDD-first, one commit each:

- **T-tui-enter-add** — `startEdit`'s refusal branch (no family, or a
  container row) delegates to `startAdd` instead of returning. The
  fallback inherits every guard: `startAdd` refuses non-addable
  sections and a schema error, and `addScope` already maps a header to
  the section's entry-level scope and a container to add-into-entry.
  Enter stays the primary action in context: a field row edits, a
  closed-set field picks, a header or container grows rows. Double-
  click on a header inherits the same fallback through the shared
  `startEdit` seam.
- **T-tui-footer-hints** — `ShortHelp` shows the focused pane's primary
  verbs, resolved through the binding table (never a second copy of the
  help text): workspace `enter · a · d · ctrl+s`, nav `enter`, plus the
  shared `· / · ? · q`. Scrolling yields the footer to the verbs — the
  arrows are universally known, enter and `a` were not. The full table
  stays under `?`, which renders from the same table unchanged.

## Acceptance

- Enter on an empty section's header opens that section's add form;
  nothing stages until the form commits.
- Enter on a structural container (a unit row) opens the add-into form
  (`add directive · demo.service`).
- The workspace footer shows `enter edit field · a add entry / apply
  detail · d remove row · ctrl+s save draft · / palette · ? help ·
  q quit`; the nav footer shows `enter open in workspace · …` and never
  hints `a` (it does nothing from the nav pane).
- `?` still lists the full per-pane table.

## Test walk (red → green)

`TestEdit_enterOnEmptyHeaderOpensAddForm`,
`TestEdit_enterOnContainerAddsIntoEntry` (both first failed with "a form
modal is open" against an empty modal stack — the silent refusal), and
`TestKeys_footerHintsShowPrimaryActions` (first failed on the old
footer: `j scroll down • k scroll up • pgup page up …`). Golden sweep
after the footer change: 33 frames, every diff the footer line and
nothing else; the narrow (60-col) frame degrades by truncation, the
bubbles help behavior.

## Verification

`go test ./... -count=1` — 20 packages ok, 0 FAIL. `go vet ./...` exit
0. gofmt clean on touched files. `golangci-lint run ./internal/tui/...`
0 issues. Each commit green on its own tree.
