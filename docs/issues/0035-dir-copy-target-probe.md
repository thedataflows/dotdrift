# ISSUE 0035: Whole-directory copy targets probe as unknown ("is a directory")

- **Type**: bug
- **Status**: open
- **Priority**: medium
- **Labels**: [status, drift, dotfiles]
- **Assignee**: none
- **Related**: [issue 0030](0030-status-multi-account-drift.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`internal/drift/drift.go`](../../internal/drift/drift.go)
- **Closing commits**: none

## Summary

A whole-directory `copy` entry
(`"~/.config/micro" = { source = "home/.config/micro", mode = "copy" }`)
reports `unknown` in `status` — the probe reads the source with
`pr.ReadFile`, which fails with `is a directory`. Status can never verify a
directory copy.

## Details

Field report: root-overlay copy entries render as
`micro: /root/.config/micro - read <dir>: is a directory (?)`.

Fix: when the copy source is a directory, compare trees: walk the source
(profile-side, always readable), read each target counterpart via the
(elevated) `ReadFile` probe, and report `ok` when every file matches,
`missing` when the target dir is absent, otherwise drift with counts
(`N file(s) missing`, `M file(s) differ`, both when mixed). Target files with
no source counterpart are not drift — copy never deletes.

## Acceptance Criteria

- [ ] Directory copy target with identical tree → `ok`
- [ ] Missing target dir → drift `missing`
- [ ] Differing/missing files → drift with counts
- [ ] Extra target-only files do not drift
- [ ] File-mode copy probing unchanged (existing tests pass)

## Out of Scope

- Whole-dir `symlink` targets (the link itself is probed today) and
  `symlink-each` (per-child, already probed)

## Notes

TDD: internal/drift tree-compare tests (equal, missing dir, differ counts,
target-only extras, unknown on unreadable target). Docs: contract #17,
cli-surface status row, log.md.
