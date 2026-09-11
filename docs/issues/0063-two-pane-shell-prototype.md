---
type: Issue
title: Two-pane shell prototype
description: Build a cheap throwaway two-pane bubbletea stub (tree + detail) to react to before the editor suite is designed.
tags: [wayfinder, prototype, tui]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0063: Two-pane shell prototype

- **Type**: task
- **Status**: done
- **Priority**: low
- **Labels**: [wayfinder:prototype]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Prototype**: branch `prototype/two-pane-shell` (6257ce5) — `internal/tui/proto/`,
  run `go run ./internal/tui/proto`; main stays prototype-free.
- **Blocked by**: [0062 TUI information architecture](0062-tui-information-architecture.md)
- **Closing commits**: pending

## Question

What does the shell decided in ticket 0062 actually *feel* like on a
terminal — is the tree/pane split, focus model, and chrome right before
any editor is designed?

- Throwaway artifact (not merged as the real TUI unless it earns it): a
  bubbletea program on the charm line decided in
  [0062](0062-tui-information-architecture.md), with the decided tree on
  the left, a detail pane on the right, the keybinding baseline, and
  fake or minimal-real data for one profile (the repo's `examples/` or
  `testdata/` profiles can feed it).
- The human reacts to: pane proportions and resize behavior, focus
  indicators, how overlay layers read in the tree, whether the chrome
  answers "where am I, what can I press".
- The prototype deliberately stubs editors and apply; it answers shell
  questions only.

Link the prototype (path or branch) from this ticket when resolved; the
reacted-upon verdict feeds ticket 0065's editor design and 0067's spec.

## Resolution

The human ran the prototype on branch `prototype/two-pane-shell` and
approved it as-is: "the branch demo looks good, proceed". All four
reaction points stand as prototyped:

- **Proportions/resize**: tree fixed at 30% (22–44 column clamp) is
  right; no proportional feedback to incorporate.
- **Focus indicators**: the focused/unfocused border-color switch
  (indigo vs dim) is enough; no extra header hint wanted.
- **Overlay reading**: overlay origins as expandable tree children
  under their module read better than inline badges; keep.
- **Chrome**: the one-line header (profile, host/user context, dirty
  indicator) plus status-bar `help.Model` hints answer "where am I,
  what can I press"; keep.

Consequence for the design set: the 0062 shell decisions (D1–D11) are
reacted-upon, not just written — no re-cutting before the editor suite.
The prototype stays on its branch as a referenced asset; main stays
prototype-free. Two deliberate simplifications are recorded so nobody
copies them from the prototype into the real TUI: static ANSI colors
(charm v2 replaces `AdaptiveColor` with `LightDark(isDark)` — the real
TUI reuses the wizard's adaptive palette), and stubbed editors/apply
(0065 and 0064 own those designs). The verdict feeds
[0065 full-schema editor suite design](0065-full-schema-editor-suite-design.md)
and [0067 assemble the TUI design set](0067-assemble-tui-design-set.md).
