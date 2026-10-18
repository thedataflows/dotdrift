---
type: Issue
title: TUI views final review fixes
description: Correct apply safety, read freshness, navigation, help, and terminal bounds after the whole-branch review.
tags: [issue, tui, service]
timestamp: 2026-10-18T14:20:00Z
---

# ISSUE 0112: TUI views final review fixes

- **Type**: bug
- **Status**: done
- **Priority**: critical
- **Labels**: [tui, service, regression]
- **Assignee**: agent
- **Related**: [TUI specification](../product/tui.md), [0110](0110-tui-apply-gate-options.md), [0111](0111-tui-generate-volumes-polish.md)
- **Related code**: [TUI](../../internal/tui/), [service](../../internal/service/), [TUI command](../../cmd/tui.go)
- **Closing commits**: `c0d9956`, `e4c83b8`

## Summary

Correct all twelve findings from the final TUI views review. An empty section
selection must not run all sections. Read views must show their data's scope and
must keep stale marks from writes that finish during a read.

## Details

The fix also prevents a second apply gate from changing a pending request. TUI
status probes must use non-interactive sudo retries. CLI status keeps its current
prompt behavior. The compositor must process write completion when a dialog is
closed or covered. Frame text and the volume picker must fit the terminal.

## Acceptance Criteria

- [x] All-off sections fail at the real service section boundary.
- [x] One apply gate, preview, and start chain owns its snapshot until it ends.
- [x] TUI sudo retries use `-n`; an expired ticket keeps the permission error.
- [x] Profile reads have a pending guard and preserve newer stale marks.
- [x] Closed or covered write dialogs and successful overrides mark read views stale.
- [x] Loaded, stale, scoped views and long footer text fit 64x24 and 100x30.
- [x] A failed Status or Plan refresh retains the previous displayed scope.
- [x] Status enter clears the nav filter and selects the winning layer when known.
- [x] The volume picker keeps its selection and volume facts visible within bounds.
- [x] Status shows the valid resume cursor or the clean resume message.
- [x] Status and Plan keep the current list visible with a refresh indication.
- [x] Add and onboard stay in Modules; apply detail and contextual help stay correct.
- [x] Full tests and vet pass. The product guide, log, and final fix report are current.

## Out of Scope

- Duplicate `elevateSlice`, old milestone comments, palette-role coverage,
  probe event cleanup, positional gate rows, and the Status detail end rebuild.
- Changes to CLI prompt policy or new key bindings.

## Notes

All changes are reversible with normal Git reverts. No persisted schema changes.
