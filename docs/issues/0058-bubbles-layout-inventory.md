---
type: Issue
title: Bubbles &amp; layout inventory
description: Survey what bubbles/lipgloss actually provide for the two-pane TUI — components, the tree gap, layout approaches, and testing — so the IA and prototype tickets decide on facts.
tags: [wayfinder, research, tui, bubbles]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0058: Bubbles &amp; layout inventory

- **Type**: task
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [wayfinder:research]
- **Assignee**: wayfinder research subagent
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [ADR-0003](../adr/0003-tui-shared-palette.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Blocked by**: none (frontier)
- **Closing commits**: none

## Question

What does the current charmbracelet stack actually give the two-pane
`dotdrift tui`, so the information-architecture and prototype tickets
decide on facts instead of folklore?

- Inventory `bubbles` components relevant to the design: `list`,
  `table`, `viewport`, `textinput`, `textarea`, `spinner`, `tabs`,
  `help`, `key`, `paginator` — for each: what it does well, what it
  lacks, API stability notes.
- The **tree gap**: bubbles has no tree component. What are the real
  options (sections-inside-`list`, community tree libs, hand-rolled
  tree over `list.Item`s), with maintenance/dependency risk for each?
- Layout: how do bubbletea apps build split panes (lipgloss
  `JoinHorizontal` + width math, charmbracelet/x exp layout helpers,
  community layout libs)? Resize handling patterns.
- Forms: what remains for `huh` inside a bubbletea app (modal dialogs,
  embedded forms) vs assembling forms from raw bubbles inputs — and how
  the generate wizard's huh usage maps onto each.
- Testing: `teatest`/`teatest/v2` state — can the design mandate driver
  tests, or should it mandate pure state machines + unit tests (the
  wizard's current pattern)?
- Version pinning: current versions of bubbletea/bubbles/lipgloss/huh in
  this repo (go.mod has bubbles as an *indirect* dep via huh) and what
  promoting them to direct requires.

Findings land in `docs/research/0058-bubbles-layout-inventory.md`,
linked from this ticket.
