---
type: Report
title: TUI views final fix report
description: Fixes, regression evidence, and final gate results for the whole-branch TUI views review.
tags: [tui, review, tests]
timestamp: 2026-10-18T14:28:00Z
---

# TUI views final fix report

All twelve findings are fixed. Issue [0112](../../../docs/issues/0112-tui-views-final-review-fixes.md)
tracks this wave. Implementation commits:
`c0d9956` (`fix(tui): make view actions safe and terminal bounds reliable`) and
`e4c83b8` (`fix(tui): reserve header width for run and draft badges`).
No subagents were used. The deferred items were not changed.

## Fixes and covering tests

The TUI regressions are in [final review tests](../../../internal/tui/final_review_test.go).
The sudo regression is in [non-interactive probe tests](../../../internal/service/probes_noninteractive_test.go).
Each row below is covered by the final full-suite command, not a filtered test run.

| Finding | Fix | Covering test | Command output summary |
|---|---|---|---|
| 1. All sections off runs all sections | The gate sends explicit false entries when no section is on. The real service `ResolveSections` returns `no sections selected`. All-on still sends nil. | `TestFinalGateAllOffRejectsAtServiceBoundary` | Red: the service returned no error. Green: full suite passed. |
| 2. Pending Preview uses shared state | A gate-chain flag blocks another apply until cancel, preview failure, or Start landing. Preview captures root and modules before the command runs. | `TestFinalApplyPendingKeepsFirstSnapshot`, `TestFinalApplyChainReleasesAfterCancelOrPreviewFailure` | Red: a second gate opened while Preview was pending. Green: confirm and decline attempts keep the first Preview and Start inputs; cancel and failure release the chain. |
| 3. A TUI probe can prompt after ticket expiry | The TUI command sets `ReadsDeps.NonInteractive`. Elevated file probes use `sudo -n`. A failed retry keeps the original permission error. CLI defaults stay unchanged. | `TestTUIProbeRetryNeverPrompts`; existing `TestElevateProbes_*` tests retain CLI coverage | Red: the sudo seam received `cat`, not `-n`. Green: valid and expired ticket cases passed; full suite passed. |
| 4. Profile refresh loses newer stale marks | Profile has a pending guard and records the stale sequence at read start. A landing read clears the mark only after success with the same sequence. | `TestFinalProfilePendingKeepsStaleMark` | Red: a second Profile read was allowed. Green: write and apply completion during a pending read keep the stale mark; the next uninterrupted read clears it. |
| 5. Write completion depends on the top dialog | The compositor owns write invalidation, before dialog delivery. Closed and help-covered dialogs no longer lose it. Successful `O` also marks read views stale. | `TestFinalWriteCompletionInvalidatesWithoutDialog`, `TestFinalOverrideInvalidatesReadViews` | Red: read views stayed fresh after detached completion and override. Green: both dialog cases and override passed. |
| 6. Frame and footer exceed terminal bounds | Long headers use the final profile directory name. The view row keeps view names, stale marks, and scoped-module counts when compact. Footer task and message text truncate. Panes have maximum dimensions. | `TestFinalChromeFitsLoadedStaleScopedViews`, `TestFinalHeaderReservesDirtyAndApplyBadges`; updated frame goldens | Red: frames reached 221 cells at widths 64 and 100. Green: all four loaded views, three stale marks, long scope names, and long error/task text fit 64x24 and 100x30. |
| 7. Failed refresh changes the retained data's scope title | Status and Plan keep pending scope separate. Only a successful read commits the displayed scope. | `TestFinalFailedRefreshKeepsDisplayedScope` | Red: both views changed from `old` to `new` after failure. Green: both retain `old`; full suite passed. |
| 8. Status enter misses a filtered owner | Jumps clear the nav filter. Known dotfile, mount, and orphan layers are selected. Dotfile lookup accepts expanded home paths. Base is the fallback; if the module has no base layer, its module row still wins over an unrelated row. | `TestFinalStatusJumpClearsNavFilter`, `TestFinalStatusJumpUsesKnownLayer` | Red: enter selected `other`; known-layer and overlay-only cases also failed. Green: all owner and layer cases passed. |
| 9. Volume picker is unbounded | Each volume has three width-bounded lines: UUID and MANAGED, type/label/size, then mountpoints. A height-based list window follows the selection. | `TestFinalVolumePickerFitsAndKeepsSelection` | Red: 80-cell rows at width 64 and 38 lines at height 24. Green: the last of 30 volumes, its 36-character UUID, and all normal volume facts remain visible at 64 and 100 columns. |
| 10. Status lacks the resume line | Status shows the service-masked cursor or the clean resume message. | `TestFinalStatusResume`; updated Status goldens | Red: no resume line. Green: pending and clean messages passed. |
| 11. Loaded refresh has no pending state | Status retains findings with `probing…`. Plan retains sections with `resolving…`. | `TestFinalRefreshKeepsListWithPendingNote` | Red: loaded Status had no pending note. Green: both lists and pending notes passed. |
| 12. Help and routing disagree | The contextual key table supplies dispatcher and help data. Add and onboard run only in Modules. Existing apply detail remains available. Status help names enter and owner-scoped apply. Status and Plan list page/home/end movement. Plan lists `d` only with the section list focused, where it is accepted; the detail pane does not accept it. | `TestFinalReadViewsDoNotEditHiddenModule`, `TestFinalHelpMatchesReadViewKeys`; existing footer and key-table tests | Red: all six hidden add/onboard cases opened dialogs, and Status help lacked enter. Green: routing, contextual help, existing key-table tests, and footer tests passed. |

## Verification

Initial full-suite runs failed on the regression tests before the related fixes.
The probe test first failed to compile against the old helper signature. It was
also checked against plain-sudo behavior after compilation: both ticket cases
failed on the missing `-n`, then the fix was restored.

A repeat gate found a header boundary case: path shortening did not reserve
space for the dirty and apply badges. The new header regression failed with a
clipped user name and hidden badges. The width calculation now includes the
badges before it chooses the compact path. The full suite then passed normally
and with `-count=2`.

A full-suite run found a snapshot instability caused by temporary path length:
the header could switch between its full and compact form. The snapshot helper
now normalizes both forms of the profile root. The frame snapshots were then
updated with all TUI tests, without a `-run` filter.

Final gate:

```text
$ GOCACHE=/tmp/gocache go test ./...
20 packages: ok
root package: [no test files]
exit 0

$ GOCACHE=/tmp/gocache go vet ./...
no diagnostics
exit 0

$ git diff --check
no diagnostics
exit 0
```

The [TUI guide](../../../docs/product/tui.md), [service API guide](../../../docs/product/service-api.md),
[issue index](../../../docs/issues/index.md), and [log](../../../docs/log.md) are updated.

## Limits and rollback

No live sudo command, password prompt, or manual terminal session was used.
Tests pin sudo arguments through the existing seam. Message-driven tests and
frame dimensions cover the TUI. No persisted schema changed; normal Git reverts
can roll back this wave.

The deferred duplicate helper, old milestone comments, palette-role test,
probe-event cleanup, positional gate rows, and Status detail end rebuild remain
out of scope.
