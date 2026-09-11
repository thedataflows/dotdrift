---
type: Issue
title: Two-pane shell prototype
description: Build a cheap throwaway two-pane bubbletea stub (tree + detail) to react to before the editor suite is designed.
tags: [wayfinder, prototype, tui]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0063: Two-pane shell prototype

- **Type**: task
- **Status**: open
- **Priority**: low
- **Labels**: [wayfinder:prototype]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Blocked by**: [0062 TUI information architecture](0062-tui-information-architecture.md)
- **Closing commits**: none

## Question

What does the shell decided in ticket 0062 actually *feel* like on a
terminal — is the tree/pane split, focus model, and chrome right before
any editor is designed?

- Throwaway artifact (not merged as the real TUI unless it earns it): a
  bubbletea program with the decided tree on the left, a detail pane on
  the right, the keybinding baseline, and fake or minimal-real data for
  one profile (the repo's `examples/` or `testdata/` profiles can feed
  it).
- The human reacts to: pane proportions and resize behavior, focus
  indicators, how overlay layers read in the tree, whether the chrome
  answers "where am I, what can I press".
- The prototype deliberately stubs editors and apply; it answers shell
  questions only.

Link the prototype (path or branch) from this ticket when resolved; the
reacted-upon verdict feeds ticket 0065's editor design and 0067's spec.
