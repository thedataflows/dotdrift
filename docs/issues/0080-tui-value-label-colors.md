---
type: Issue
title: TUI color semantics — values read as content, fixed labels recede
description: Dialog and form rows split their fixed label (muted) from their value (bright neutral), placeholder hints stay dim, and the value hue rides rowText so picker, nav, and workspace rows recolor with it.
tags: [issue, tui, ergonomics, color]
timestamp: 2026-09-21T13:00:00Z
---

# ISSUE 0080: TUI color semantics — values vs fixed labels

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ergonomics, color]
- **Assignee**: none
- **Related**: [0079](0079-tui-undo-marks-onboard-cycle.md), [0075](0075-tui-ergonomics-paging-selection-disclosure.md)
- **Related code**: [`internal/tui/theme.go`](../../internal/tui/theme.go), [`internal/tui/dialog.go`](../../internal/tui/dialog.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go), [`internal/tui/choices.go`](../../internal/tui/choices.go)
- **Closing commits**: c40cefe

## Summary

Values and fixed labels rendered in the same default foreground almost
everywhere. Worst in dialogs: `dlgRow.render` concatenated
`label + " " + value` as one plain string, so in the onboard form the
label, the typed path, and the `(detected account)` hint were
indistinguishable until a row was focused. User ask: differentiate
values from fixed labels by color across all dialogs and screens.

## Details

One task, TDD-first (**T-tui-value-color**):

- **Two new registry entries, no inline styles** (ADR-0003 discipline):
  the palette gains a `value` hue — a bright neutral resolved per
  background (`255` on dark, `234` on light; the brightest chrome on
  screen, deliberately not a fifth accent hue so indigo/fuchsia/red/amber
  keep their semantics) — and the theme gains `fieldLabel` (muted, the
  fixed-label style) while `rowText` becomes the value/content carrier
  (foreground `value`; it keeps its truncation role).
- **Dialog and form rows** (`dlgRow.renderRow`, shared by the onboard,
  restore, generate, add, and move-to forms): the label renders in
  `fieldLabel`, the value in `rowText`, an empty field's placeholder hint
  in `disabledMark` (dim) — filled vs unfilled reads by color, backed by
  the hint's parentheses so it is never color-only. A choice row's value
  is bright and its `(< > to change)` affordance stays dim. `render` was
  dissolved into `segments` (label, value, hint); the focused row keeps
  the 0075 cursor bar wrapped around the styled segments (nesting was
  already proven by the in-row marks).
- **Every content surface recolors through the carriers**: the pickers
  and the workspace and nav row renderers already go through `rowText`,
  so they take the value hue without further code. Chrome is untouched —
  footer, meta, modal titles, run output, and the apply modal's log stay
  default/dim: output is not editable data.

Deliberately rejected: a cyan/teal value hue (louder, but a fifth accent
competing with the existing four) and styling the palette's action rows
(actions are labels, not values).

## Acceptance

- The palette resolves `value` to `255` dark / `234` light, and the
  value hue differs from muted and dim on both backgrounds
  (`TestTheme_lightDarkPalette`, extended).
- `fieldLabel` and `rowText` renders differ visibly on both backgrounds,
  and the value style is visible against the default foreground
  (`TestTheme_valueVsLabel`).
- A dialog row renders its label in `fieldLabel`, its typed value in
  `rowText` (never dim), an empty field's hint in `disabledMark` (never
  value-styled), a choice's value bright with the cycle affordance dim,
  and the focused row keeps the `│` bar around the styled segments
  (`TestDlgRow_labelValueHintColors`).

## Verification

Fresh runs on the final tree: `go test ./... -count=1` — all packages
ok, 0 FAIL; `go vet ./...` exit 0; gofmt clean; golangci-lint
`run ./internal/tui/` — 0 issues. Golden sweep: no golden changed and
none needed to — the golden helper strips ANSI by design, so the layout
is byte-identical and the color layer is pinned by the ANSI-segment
tests instead (asserted with `Contains` on the exact registry renders).
Ponytail audit: one palette hue, one new style, one layout-function
split; every other surface recolors through the two existing carriers —
no per-surface styling code, nothing to cut.
