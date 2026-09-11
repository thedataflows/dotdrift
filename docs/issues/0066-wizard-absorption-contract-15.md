---
type: Issue
title: Wizard absorption &amp; contract #15 amendment
description: Decide what happens to generate --tui once the TUI covers mounts/smb, and draft the contract/ADR edits the absorption requires.
tags: [wayfinder, task, tui, contract]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0066: Wizard absorption &amp; contract #15 amendment

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:task]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariant 15](../product/contract.md), [M13 Generate](../milestones/m13-generate.md)
- **Related code**: [`cmd/generate.go`](../../cmd/generate.go), [`internal/tui/`](../../internal/tui/)
- **Blocked by**: [0065 Full-schema editor suite design](0065-full-schema-editor-suite-design.md)
- **Closing commits**: none

## Question

The TUI absorbs the `generate --tui` wizard. What is the concrete
amendment plan?

- Contract 15 today pins CLI/wizard byte-equivalence via shared assembly
  helpers. After absorption, the invariant must say something like
  "CLI mode and the TUI editors assemble `generate.Input` through the
  same shared helpers" — draft the exact replacement wording, and check
  whether any other invariant (12, 13, 19) needs touching.
- The equivalence test story: `cmd/generate_tui_equivalence_test.go`
  asserts byte-identical trees; what is its successor once the wizard's
  huh front-end is gone and the TUI editors are the interactive path?
- CLI surface: does `generate --tui/--no-tui` disappear (so `generate`
  becomes CLI-only and interactivity lives in `tui`), and what happens
  to the no-flags-no-terminal actionable error path?
- Sequencing: can the wizard die in the same release the TUI ships, or
  is there a deprecation window? What marks it in docs (README, cli
  surface, M13's text)?
- ADR need: wizard absorption + the service-layer doorway are candidate
  ADRs (hard to reverse, surprising later, real trade-offs). Decide
  which ADR(s) the design set carries and draft their skeleton here so
  ticket 0067 assembles finished text.
