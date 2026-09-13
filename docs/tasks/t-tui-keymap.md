---
type: Task
title: T-tui-keymap
description: The single keymap — one binding table, shift-for-dangerous, one esc meaning, contextual ? help, mouse parity; old per-view key handling deleted.
tags: [task, tdd, tui, keymap]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Make the M15 keymap a designed set instead of an accumulation, per
[M15](/milestones/m15-tui-compositor.md): one binding table, one meaning
for `esc`, shift-for-dangerous, contextual help. This task deletes the
old per-view key routing once the new surfaces (tasks 1–6) carry the new
bindings.

# Tests first

- Message-driven: `TestKeys_table` — the whole base map as one table
  test: `j`/`k`/arrows move, `h`/`l` collapse/expand,
  `tab`/`shift+tab` focus, `enter`/`e` edit, `esc` pops top, `/`
  palette, `n` new module, `a` add row, `d` remove, `D` discard draft,
  `ctrl+s` save, `w` writes, `m` manage, `p` plan, `P` apply, `L` layer
  cycle, `?` help, `q` quit (dirty-confirm).
- `TestKeys_shiftIsTheDangerousVersion` (`p`/`P`, `d`/`D` pairs),
  `TestEsc_oneMeaning` (modal → edit → focus-to-nav, in that order, from
  every surface), `TestHelp_contextual` (`?` lists only bindings valid
  right now — editing shows edit keys, modal open shows modal keys),
  `TestKeys_modalOverrides` (palette captures all but `esc`/`ctrl+n`;
  elevation all but `esc`; confirm all but `y`/`n`/`esc`),
  `TestMouse_parity` (click selects/focuses, double-click edits, wheel
  scrolls the hovered pane, modal buttons clickable — nothing mouse-only).
- `TestKeys_noOrphanedBindings` — every binding in the table is wired;
  every wired binding is in the table (the table is the source of truth).

# Implementation notes

- No undo: drafts + confirms are the safety net; a real undo across file
  writes is scope creep with a cape.
- `P` for apply keeps the plan summary + elevation flow from
  T-tui-modals; `p` (plan) never prompts for credentials.
- The binding table lives in one file as data (key, action, contexts,
  help text); the help modal and footer hints render *from* it, so docs
  cannot drift from behavior.

# Docs

- tui.md keymap section replaced by the generated table; README key
  summary updated; log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
