---
type: Task
title: T-tui-structural-when
description: Nested when groups (and/or/not) become an always-expanded editable tree; root leaves always render so unset conditions become settable; group add/remove with empty-group cross-checks.
tags: [task, tdd, tui, editing, issue-0074]
timestamp: 2026-09-15T00:00:00Z
issue: "0074"
---

# Goal

Third 0074 slice: the when tree (0065-D12's builder, landing as rows).
Root leaves always render (an unset `gpu` becomes editable — today it
cannot be set at all); `and[i]`/`or[i]`/`not` render as expanded,
indented group containers whose leaves edit like root leaves; groups add
(`a` → and/or/not, nested at the cursor's group) and remove with
confirm.

# Tests first

- Rows: root leaves always present (unset dim), group containers carry
  machine paths (`and`, `0`, `or`, `1`, `hosts`…), nesting indents.
- Edits: leaf fields at depth (comma lists, gpu/kernel scalars); editing
  a group leaf re-encodes the whole when family.
- Adds: `a` with input and/or/not (tier-1 otherwise) appends to the
  cursor row's group list / sets not; nested building round-trips.
- Removes: group container d + confirm (indices shift — dirty markers
  are positional, documented); crossCheck refuses empty groups (nothing
  to evaluate) at save, matching the load error's semantics.
- Goldens: root-with-groups, nested tree.

# Implementation notes

- When row keys grow segments under the existing `FamilyWhen` family;
  top-level leaf keys stay exactly as today (compat).
- Group add keys are positional (`structuralNewKey` predicts the
  appended index); addPath scopes the target group.

# Docs

- tui.md editing bullet (the when tree), log entry, issue 0074 checkbox.

# Acceptance

- [x] Landed — commit `feat(tui): 0074 when-tree editing`.
- [Definition of done](/engineering/definition-of-done.md) checklist complete.
