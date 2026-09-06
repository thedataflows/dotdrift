# ISSUE 0029: Surface superuser-owned user overlays when running unprivileged

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [profile, selection, ux]
- **Assignee**: none
- **Related**: [merge rules](../product/merge-rules.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`internal/profile/`](../../internal/profile/), [`cmd/`](../../cmd/)
- **Closing commits**: 37533fa

## Summary

A module placed under `users/<uid-0 account>/modules/` (e.g. `users/root`)
is invisible to unprivileged runs: user layers key on the current OS account,
so `modules`, `plan`, `status`, and `apply` select nothing from it and —
worse — say nothing about it. Surface such overlays as skipped with an
actionable reason instead of silently omitting them.

## Details

Field report: a module added under `users/root/` showed nothing under any
command. Selection-by-current-account is a documented product invariant
([merge rules](../product/merge-rules.md): "Under `sudo dotdrift apply` the
process account is root, so root's overlays are selected"), so the behavior
is by design — the silence is the defect.

Decisions (wayfinder session 2026-09-03, user-accepted recommendations):

- An overlay owner is a **superuser** when its OS account has **uid == 0**,
  resolved via `os/user.Lookup(<dirname>)`; lookup failure means "not a
  superuser". gid==0 (root-group membership) is NOT the test — the report's
  "gid=0" phrasing was sharpened during the session.
- Discovery surfaces superuser-owned user-layer modules as **skipped** with
  reason `requires root (run with sudo)`. `modules` renders them natively in
  its skipped list; `plan`/`status`/`apply` (the shared `loadAndResolvePlan`
  preamble) emit one stderr warning naming the count. `restore` stays quiet
  (it reads backup indexes, not the selection).
- Selection, resolve, and apply semantics do not change: running with sudo
  remains the only way to select these overlays.

Implementation shape: after `Select`, `Load` scans `users/<name>/modules/`
for every `<name>` that is not the current username fact and resolves to a
uid-0 account, and appends those modules to `Profile.Skipped` with the
reason above. They never enter `Profile.Modules`, so when-filters, probes,
and resolve are untouched.

## Acceptance Criteria

- [ ] Unprivileged `dotdrift modules` lists `users/root` modules as skipped
      with reason `requires root (run with sudo)`
- [ ] `dotdrift plan`/`status`/`apply` emit one warning when such overlays
      exist and are unselectable
- [ ] Running as root (username fact = the uid-0 account) selects the
      overlay normally — no skip entry
- [ ] A user-layer dir whose owner account does not exist, or whose uid is
      not 0, stays invisible (unchanged behavior)
- [ ] The test covers a uid-0 account named something other than `root`

## Out of Scope

- Self-elevation or re-exec under sudo — explicit `sudo dotdrift …` stays
  the model
- Selecting superuser overlays from an unprivileged run (a selection change)
- `hosts/` overlays for other hosts — invisible by design, unchanged

## Notes

TDD: `internal/profile` tests for the skip pass (uid-0 surfaced, root-run
selects, lookup-failure invisible, non-root-owner invisible, non-root uid-0
name) and a `cmd` test for the warning emission. Docs: merge-rules sudo
paragraph, cli-surface `modules` row, CONTEXT.md glossary term, log.md.
