---
type: Issue
title: Field-edit caret reset strips the row color from the tail
description: In the inline field editor the text lost its accent color when the caret moved left into it and regained it at the end; the caret's full ANSI reset erased the enclosing row style for the rest of the line.
tags: [issue, tui, editing]
timestamp: 2026-09-22T00:00:00Z
---

# ISSUE 0102: TUI field-edit caret reset strips the row color from the tail

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, editing]
- **Assignee**: none
- **Related**: [0092](0092-tui-field-caret-glyph-shifts-text.md)
- **Related code**: [`internal/tui/theme.go`](../../internal/tui/theme.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go)

## Summary

Editing a package line (or any inline field), the text renders in the
cursor row's accent color — but moving the caret left into the text lost
the color, and moving it back to the right end brought it back. The
caret must never alter the text color.

## Details

`editLines` nests the 0092 block caret inside the cursor-row render:

```go
shown := string(runes[:cur]) + th.caret.Render(at) + after
lines := []string{th.cursorRow.MaxWidth(width).Render(" ▸ " + shown)}
```

lipgloss closes every styled run with a full reset (`ESC[m`), so the
rendered line is

```
ESC[1;95m ▸ ab ESC[7mc ESC[m de ESC[m
```

The caret's own reset kills the row's bold+fuchsia for everything after
it. With the caret at the end of input the reset trails the last cell
and nothing follows it, so the color survives — the reported
"left loses it, right brings it back".

The fix is at the shared point, not the call site: the caret is rendered
by a new `theme.caretCell`, which swaps the caret style's closing full
reset for `ESC[27m` (reverse off). The caret releases only the attribute
it set; the enclosing row's style carries on after it. All three caret
render sites route through it — the workspace field editor (the reported
case), the multi-line editor's caret line (plain there, behavior
unchanged), and the picker's location bar (caret last, unchanged) — so
the invariant "the caret never resets the style around it" holds for
every future nesting too.

## Acceptance criteria

- [x] With the caret mid-text, the tail after the caret keeps the row's
  accent — no full reset between the caret cell and the tail.
- [x] The caret is still a reverse-video block on the cell under it
  (0092): `ESC[7m` on, `ESC[27m` off, never an inserted glyph.
- [x] All caret render sites (`editLines`, the multi-line editor, the
  picker location bar) use `theme.caretCell`.

## Test plan

- `TestTheme_caretCell` pins the unit invariant: reverse on, reverse
  off, no full reset (`ESC[m` or `ESC[0m`) in the cell.
- `TestEdit_caretKeepsTailColor` renders a mid-text caret and asserts
  the tail follows `ESC[27m`, never a full reset.
- The 0092 assertions in `TestEdit_caretIsBlockNotGlyph` now pin on
  `caretCell` (the block treatment and the end-of-input caret cell).

## Out of scope

- Dialog form fields still render no caret at all (noted in 0092 as a
  deliberate follow-up); when they grow one, it rides `caretCell`.

## Notes

lipgloss v2 emits the reset as `ESC[m` (empty parameter); `caretCell`
accepts the explicit `ESC[0m` spelling too, so a dependency upgrade
cannot silently reintroduce the bug — `TestTheme_caretCell` would go
red.
