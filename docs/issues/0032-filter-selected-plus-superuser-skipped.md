# ISSUE 0032: Module filter rejects a selected module that also has a superuser skip entry

- **Type**: bug
- **Status**: open
- **Priority**: medium
- **Labels**: [profile, filter, selection]
- **Assignee**: none
- **Related**: [issue 0029](0029-superuser-user-overlay-visibility.md), [issue 0030](0030-status-multi-account-drift.md)
- **Related code**: [`internal/profile/filter.go`](../../internal/profile/filter.go)
- **Closing commits**: none

## Summary

`dotdrift status --profile <p> micro` errors with
`module(s) not selected: micro (requires root (run with sudo))` even when
`micro` IS selected in the invoking view (base layer) — because the
superuser-overlay skip entry (issue 0029) for the same module id lands in
`Skipped`, and `LimitTo` rejects any id with a skip reason without checking
current selection.

## Details

Field report: base `modules/micro` selected, `users/root/modules/micro`
surfaced as skipped by 0029's `markSuperuserOverlays`. Both share the module
id `micro`. `LimitTo` builds `skipReasons` from `p.Skipped` and errors on the
id before looking at `p.Selected`. Pre-0029, `Selected` and `Skipped` were
disjoint over `p.Modules`, so "skipped ⇒ not selected" held; 0029 broke the
invariant by appending skip entries for modules outside `p.Modules` whose ids
may collide with selected ones. The skip entry describes the other account's
overlay copy, not this view's selection.

Fix: a filter id that is currently selected passes. A filter id naming a
skipped-and-not-selected module errors carrying the skip reason — and this
now covers ids absent from `p.Modules` too (an overlay-only superuser module
errors "not selected: <id> (requires root…)" instead of the misleading
"unknown module(s)").

## Acceptance Criteria

- [ ] Filtering by a module that is selected AND superuser-skip-listed
      (another account's overlay) passes and keeps it selected
- [ ] Filtering by a module that exists ONLY in a superuser overlay errors
      "module(s) not selected: <id> (requires root (run with sudo))" — never
      "unknown module(s)"
- [ ] Existing filter behavior unchanged: unknown ids error, disabled/when-
      skipped ids error with their reason, selection shrinks for the rest

## Out of Scope

- Status tolerating filters that name other-account-only modules (report
  the invoking view as empty and still render their sections) — a deliberate
  product decision, not this bug; candidate for a separate issue if wanted

## Notes

TDD: `internal/profile` filter tests for the selected-plus-skip-listed pass
and the overlay-only not-selected error wording. Docs: cli-surface module
filter section, log.md.
