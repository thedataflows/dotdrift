---
type: Issue
title: "0141: Modules panes carry one leading space too many"
description: Bug — in the TUI Modules view, nav items and workspace section labels start one space too far right; the Plan view's cursor pipe sits directly next to the label, and Modules must match.
tags: [issue, bug, tui]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0141: Modules panes carry one leading space too many

- **Type**: bug
- **Status**: open
- **Priority**: low
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0116](0116-section-bar-alignment.md)
- **Related code**: [`internal/tui/nav.go`](../../internal/tui/nav.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go)
- **Closing commits**: none

## Summary

User report from screenshots: in the Plan view the selection pipe
sits directly next to the section label (`│tools: 3`), and that is
the wanted look. In the Modules view, both the left panel's items
and the right panel's section labels carry one extra leading space
(`│ ▸ shell`, `│ PACKAGES`), so text starts one column too far
right and shifts when the cursor arrives.

## Details

Remove one leading space, keeping every variant of a row aligned
with its siblings (text still starts in one shared column per pane):

- Nav rows (`navModel.rowView`): cursor rows render
  `cursorLine(rest)` instead of `cursorLine(" "+rest)`; plain rows
  render `" "+rest` instead of `"  "+rest`. Child (layer) rows keep
  their relative indent.
- Workspace section labels (`workspaceModel.rowView` header case):
  cursor bar `cursorLine(text)` instead of `cursorLine(" "+text)`,
  filled bar `barLine(text)` instead of `barLine(" "+text)`, plain
  heading `" "+text` instead of `"  "+text`. In all three variants
  the label text then starts in the same column, right after the
  cursor pipe / bar edge.

Workspace item rows, the inline editor row, and the other views
(Status already trims its cursor lead) are unchanged.

## Acceptance Criteria

- [ ] Nav cursor row renders `│▸ shell` (pipe next to the marker);
  plain nav rows start with one space.
- [ ] Workspace section labels render `│PACKAGES` under the cursor;
  filled-bar and plain labels keep the same text column.
- [ ] TUI goldens are regenerated and reviewed; no other view's
  spacing changes.

## Out of Scope

- Item rows in the workspace and the Plan/Status/Profile panes.
