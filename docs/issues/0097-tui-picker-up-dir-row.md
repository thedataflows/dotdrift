---
type: Issue
title: TUI file picker lists a ../ up-dir row whenever the directory has a parent
description: The only ways out of a directory were the left/h/backspace chord and the location bar. The listing's first row is now ../ — enter and right on it navigate up like left (never pick, not even in dirs mode), home and wheel-up reach it, and the default cursor still opens on the first real entry.
tags: [feature, tui, dogfooded]
timestamp: 2026-09-22T00:00:00Z
---

# ISSUE 0097: TUI file picker lists a ../ up-dir row whenever the directory has a parent

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, dogfooded]
- **Assignee**: none
- **Related**: [0094](0094-tui-typed-inputs-multiline-filepicker.md) (the picker), [0096](0096-tui-picker-opens-on-current-value.md) (the seed selection the default cursor preserves)
- **Related code**: [`internal/tui/filepicker.go`](../../internal/tui/filepicker.go)

## Summary

Dogfooded request: the file/dir picker should have an up-dir entry if
there is a parent. Until now the only ways toward the filesystem root
were the `left`/`h`/`backspace` chord and the `ctrl+l` location bar —
the listing itself offered no way up, unlike every desktop file dialog,
where `..` is the first row.

## Details

The `../` row is a view-and-navigation concept, not a listing entry:
`visible()` still returns real entries only, so every existing listing,
filter-count, and seed-selection assertion is untouched. The cursor
gains a `-1` sentinel one notch below the first real entry:

- **Render** — a pinned `../` line above the listing (dirs styling,
  cursor-row styling when selected), present whenever
  `filepath.Dir(cwd) != cwd`. It stays put while the `/` filter
  narrows or empties the listing, and it takes one line of the list
  height, so the paging step shrinks by one.
- **Activation** — enter and right on it call the same `goUp` as
  left/h/backspace, in **every** mode: in dirs mode it navigates, it
  never picks the parent (picking the shown directory remains
  `ctrl+enter`).
- **Reach** — `home`/`g` goes to the very top (the `../` row),
  `up`/`k` and wheel-up from the first entry land on it, and an
  emptied listing (no entries, or a filter with no matches) selects
  it, so enter still does something useful.
- **Preserved behavior** — the default cursor still opens on the
  first real entry (0096's seeded selection included), so an
  unseeded enter descends into the first directory exactly as before;
  at the filesystem root there is no parent and no row.

## Acceptance criteria

- [x] A directory with a parent lists `../` as its first row; enter
      and right on it navigate up and never pick (dirs mode included).
- [x] The filesystem root lists no `../` row; home clamps to the first
      entry there.
- [x] The default cursor and the 0096 seed selection are unchanged —
      both land on real entries, never on `../`.
- [x] The row survives filtering and empty listings; paging steps
      account for it.
- [x] `go test ./...` and `go vet ./...` green.

## Implementation notes

TDD red→green: 6 new tests failed first (home clamped at 0, no `../`
in the render, wheel-up pinned at the first entry) plus 2
characterization pins on preserved behavior (default cursor skips the
row; the root has none). `TestPicker_pagingAndEnds`'s top-of-list
positions shifted by one — the intentional behavior change, its
assertions rewritten to name the `-1` up-dir row. The GREEN stayed
inside `filepicker.go`: `hasUp`/`goUp` helpers, a `-1` floor in
`clampSel`, the sentinel cases in `enter`/`descend`/nav keys, and the
pinned row in `view`. No call-site or compositor wiring changed.
