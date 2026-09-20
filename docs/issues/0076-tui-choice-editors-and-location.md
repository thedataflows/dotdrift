---
type: Issue
title: TUI editing round — choice pickers for closed-set fields, cursor location you can see
description: Fields with a known value set open a picker instead of the free-text input, and selectable section headers show the cursor bar plus a persistent section indicator.
tags: [issue, tui, ergonomics, follow-up]
timestamp: 2026-09-17T00:00:00Z
---

# ISSUE 0076: TUI editing round — choice pickers for closed-set fields, cursor location you can see

- **Type**: task
- **Status**: done
- **Closing commits**: 1c297b5, 63d3fff
- **Follow-up to**: [0074](0074-structural-section-editing.md), [0075](0075-tui-ergonomics-paging-selection-disclosure.md)

## Context

Two dogfooding findings on the compositor workspace:

1. **Free-text editors where a selection fits.** Fields whose value set is
   closed — `scope` (`user`/`system`), mount `state`
   (`enabled`/`disabled`), the boolean fields (`allow_empty`,
   `writable`, `public`), smb `avahi` (true/false/unset), and the systemd
   `Type`/`Restart` directives — open the free-text input today. A picker
   is faster, cannot produce a tier-1 typo error, and shows the valid
   values instead of hiding them in an error message.
2. **No visible location.** Selectable section headers (an empty
   section's header is the way in; the when/smb headers are those
   sections' root scope) render through the header branch of the
   workspace row renderer, which returns before the cursor treatment is
   applied — walking down onto an empty `writes`/`when`/`hooks` section
   shows no cursor at all, and nothing else names the section under the
   cursor.

## Details

Two tasks, TDD-first, one commit each:

- **T-tui-choice** — `enter`/`e` on a row whose field has a closed value
  set opens a centered choice picker modal instead of the inline input:
  one row per value, the effective value marked, arrows (and j/k and
  left/right) walk, enter picks, esc cancels, click selects, clicking
  the selected row picks. Picking the effective value closes without
  staging anything (editing `scope` from its unset default to `user` is
  a no-op, not a diff). Every other field keeps the free-text input.
- **T-tui-location** — the header branch applies the `cursorRow`
  treatment when the cursor rests on the header, and the workspace title
  line names the cursor's section (`demo  [user] · writes`) so the
  location is always visible, not only when the bar is on screen.

## Acceptance Criteria

- [x] scope, mount state, the three boolean fields, avahi, and systemd
      Type/Restart open the picker; committing a pick splices the same
      draft path as a text edit (dirty marker, row re-render, save)
- [x] picking the current value stages nothing; esc stages nothing
- [x] free-text fields (description, app, packages, hooks, links,
      sources, when values, directive values outside Type/Restart) are
      unchanged
- [x] a selectable header under the cursor renders the bar; the title
      line shows `· <section>` wherever the cursor rests
- [x] `?` over the picker shows its keys; the full suite stays green
