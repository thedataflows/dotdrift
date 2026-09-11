---
type: Issue
title: Bubbles &amp; layout inventory
description: Survey what bubbles/lipgloss actually provide for the two-pane TUI — components, the tree gap, layout approaches, and testing — so the IA and prototype tickets decide on facts.
tags: [wayfinder, research, tui, bubbles]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0058: Bubbles &amp; layout inventory

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:research]
- **Assignee**: wayfinder research subagent
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [ADR-0003](../adr/0003-tui-shared-palette.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Blocked by**: none (frontier)
- **Closing commits**: pending

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

## Resolution

Findings: [Bubbles & layout inventory](../research/0058-bubbles-layout-inventory.md).

- The charmbracelet **v2 generation is GA** (bubbletea v2.0.0 on 2026-02-24,
  now v2.0.9; bubbles v2.2.1; lipgloss v2.0.6; huh v2.0.3) under new
  `charm.land/*` module paths (Go 1.25+; this repo is go 1.26). The pinned v1
  line is maintenance-mode — bubbles v1.0.0 is an explicit "honorary release".
  v1-vs-v2 is now the first design decision for the IA/prototype tickets.
- The **tree gap is closed on v2 only**: bubbles v2.2.0 (2026-08-21) shipped
  an official tree component; on v1 there is no tree, but lipgloss v1.1.0
  (already pinned) ships the render-only tree, leaving only
  selection/scrolling to hand-roll. Community tree libs are not viable
  dependencies at any stack level.
- **Component inventory**: bubbles v1 has 15 packages (+ `tree` on v2); there
  is no `tabs`, `form`, or `layout` component upstream and there never was a
  merged tabs proposal — tabs stay hand-rolled lipgloss (as `chrome.go`
  already does); forms are huh's job. list/table/viewport capabilities and
  caveats are documented per component in the research doc.
- **Two-pane layout needs no new dependency**: Width/Height-styled panes +
  `lipgloss.JoinHorizontal`/`JoinVertical`, with `tea.WindowSizeMsg` →
  recompute → `SetSize` as the resize pattern (gh-dash is the reference
  implementation); lipgloss gotchas (Width/Height are minimums, `Place*`
  no-ops when oversized, border budgeting via `GetFrameSize`) are recorded.
- **Forms**: embedding a huh form in a larger bubbletea app is the officially
  example-backed pattern (form as child model beside a pane); pane-embedded
  editors are likelier raw bubbles inputs. **Testing**: teatest (v1 and v2)
  remains untagged/experimental, so the mandate stays pure state machines +
  unit tests (the wizard's pattern) plus `View()` golden-file tests via
  x/exp/golden. The wizard's pure decision functions carry over unchanged.
