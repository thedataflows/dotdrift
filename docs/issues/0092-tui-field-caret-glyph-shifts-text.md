---
type: Issue
title: Field editor caret glyph occupies a cell — letters right of the caret shift
description: The workspace field editor renders its caret by inserting the ▏ glyph into the text; the glyph takes a terminal cell, so moving the caret mid-text pushes the remaining letters one cell right, producing a visual spacing discrepancy.
tags: [bug, tui, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0092: Field editor caret glyph occupies a cell — letters right of the caret shift

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0075](0075-tui-ergonomics-paging-selection-disclosure.md) (selection/cursor treatments)
- **Related code**: [`internal/tui/workspace.go`](../../internal/tui/workspace.go) (`editLines`), [`internal/tui/theme.go`](../../internal/tui/theme.go)

## Summary

Found dogfooding: while editing a field in the module details view,
moving the caret left/right adds a space-like gap at the caret position,
so the letters right of the caret sit one cell further right than they
do when the caret is elsewhere — a visible spacing discrepancy inside
words.

## Details

`editLines` renders the caret by splicing the `▏` glyph into the buffer
text: `runes[:cur] + "▏" + runes[cur:]`. The glyph occupies a terminal
cell like any character, so it is not a caret *on* the text but a
character inserted *into* it — the tail shifts right by one cell
wherever the caret rests.

The fix is the block caret: the character under the caret renders with
reverse video (a new `caret` style in the theme registry — one style
per visible concept, never an inline chain), and at end of input a
reversed trailing space marks the position. No glyph is inserted, so
the stripped line always equals the buffer text and letters never move.

Out of scope: the dialog form fields (`dlgField`) support left/right
caret movement but render no caret at all — blind movement. A visible
block caret there is a deliberate follow-up, not part of this bug.

## Acceptance Criteria

- [x] A mid-text caret adds no cell: the stripped edit line equals the
  buffer text
- [x] The character under the caret renders with the reverse treatment;
  end-of-input renders a caret cell
- [x] No `▏` glyph remains in the field editor's rendering
- [x] `go test ./...` passes

## Notes

Fixed in the closing commit below.
