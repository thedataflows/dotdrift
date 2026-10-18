---
type: Issue
title: "0140: Plan detail pane repeats the section header"
description: Bug — the TUI Plan view's detail pane shows the section name twice, once as its header strip and once as the report block's own header line.
tags: [issue, bug, tui, plan]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0140: Plan detail pane repeats the section header

- **Type**: bug
- **Status**: done
- **Priority**: low
- **Labels**: [tui, plan]
- **Assignee**: none
- **Related**: [0109](0109-tui-plan-view.md)
- **Related code**: [`internal/tui/planview.go`](../../internal/tui/planview.go)
- **Closing commits**: `0cce403`

## Summary

In the Plan tab, the detail pane names the selected section twice:
once in its own header strip and once as the first body line, which
is the report block's own `<section>:` header from
`RenderPlanReport` output. Seen on the user's screen: a filled
`packages` strip, then a plain `packages:` line, then the entries.

## Details

`planDetail` prepends the section header strip, then appends
`planSectionLines`, which cuts blocks out of the rendered report
with `reportBlock` — and a block includes its own header line.
The fix: `planSectionLines` drops the leading line when it is the
section's own header (`<sec>:`). Sub-block headers inside a section
stay: the packages section still shows its `remove:` sub-header,
which separates the install list from the remove list.

## Acceptance Criteria

- [x] The Plan detail pane shows the section name once (the header
  strip); no `<section>:` line follows it in the body.
- [x] The packages section keeps its `remove:` sub-header.
- [x] Plan view goldens are regenerated and reviewed.

## Out of Scope

- Changing `RenderPlanReport` itself: the CLI report keeps its
  section headers.
