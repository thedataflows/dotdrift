---
type: Issue
title: Module-management dialogs in the TUI
description: The config area's module ops (create scaffold, move-refuse, delete with orphan preview) get their context-menu dialog UI in the shell, plus the esc-back affordance the writes dialogs never had.
tags: [implementation, tui, editors]
timestamp: 2026-09-13T00:00:00Z
---

# ISSUE 0072: Module-management dialogs in the TUI

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: dev
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0065 full-schema editor suite](0065-full-schema-editor-suite-design.md), [tui spec](../product/tui.md), [M14](../milestones/m14-tui.md)
- **Related code**: [`internal/service/config.go`](../../internal/service/config.go), `internal/tui/`
- **Blocked by**: none (the service ops shipped with T-tui-editors)
- **Closing commits**: this commit

## Question

The M14 exit criteria name "module management dialogs (create scaffold,
move-refuse, delete with orphan preview)" and [tui.md](../product/tui.md)
specs them as context-menu dialogs over the config area. The service ops
(`CreateModule`, `MoveModule`, `DeletePreview`, `DeleteModule`) shipped
with T-tui-editors and are tested, but no TUI code references them: the
dialog UI was deferred "until T-tui-writes adds their UI" and task 4
shipped only the Profile dialogs. The milestone exit criterion is
therefore unmet. Found during the milestone exit-criteria audit.

While wiring the entry point, a second gap surfaced in the shared dialog
branch: the writes dialogs render the hint "esc back" but the shell never
routes `esc` to a pop there, so an onboard/restore/generate dialog cannot
be left without finishing it. The same branch serves the new dialogs, so
the fix lands here.

## Approach

- Widen the `service.ConfigEditor` doorway with the four module ops
  (additive; `*ConfigArea` already implements them, editor frames keep
  their narrow `editor.Config` view of the same area).
- One `manageDialog` in `internal/tui/manage.go`: a context menu
  (create / move / delete, entries that need a selected module hide when
  the selection carries none) whose entries open in place as forms over
  the same dialog. Delete pre-computes the orphan preview before the
  confirm gate; move renders the typed collision refusal; create
  scaffolds the minimal module. Success reloads the modules read so the
  tree reflects the change.
- Entry: `m` on a module or origin selection (the tree position is the
  context), mirroring `e` for editors.
- The dialog interface gains `back() bool` — the esc discipline: a
  confirm gate consumes esc (clears, stays), a form steps back to the
  menu, a bare menu lets the shell pop.

## Evidence

TDD red for every behavior before implementation; `go test ./...`,
`go vet ./...`, golangci-lint v2, the offline vendor suite, and
`-race` on the touched packages green at the closing commit.
