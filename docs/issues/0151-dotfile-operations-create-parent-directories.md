---
type: Issue
title: 0151 Dotfile operations create parent directories
description: Every dotfile operation creates the target parent directory when it does not exist.
tags: [issue, dotfiles, apply]
timestamp: 2026-08-03T00:00:00Z
---

# ISSUE 0151: Dotfile operations create parent directories

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [dotfiles, apply]
- **Assignee**: none
- **Related**: [0042](0042-system-files-bootstrap-files.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`internal/service/steps.go`](../../internal/service/steps.go)
- **Closing commits**: none

## Summary

Every dotfile operation must create the parent directory of its target
when it does not exist, for every mode and every scope. Today this is not
guaranteed: system-scope entries converge through mise
`[bootstrap.files]`, which fails with "managed file parent does not
exist" when the parent is missing (verified against mise 2026.11.8 for
copy and template entries).

## Details

Checked against mise 2026.11.8:

- User scope (`mise dotfiles apply`, `[dotfiles]`): copy, symlink,
  template, symlink-each, and line edit entries create missing parents.
  dotdrift still creates the parents itself before the apply, so the
  guarantee does not depend on the mise version.
- System scope (`mise bootstrap --only files`, `[bootstrap.files]`):
  no mode creates missing parents. The fix emits each target's parent as
  a `[bootstrap.directories]` entry in the same config, so mise creates
  it in the same run, with its normal privilege handling. This also
  covers system edit entries: their file's parent joins the same
  directories list, and the bootstrap run happens before the elevated
  `[dotfiles]` edit apply.

Parent means: the directory that holds the target file. For an edit
entry, the directory that holds the edited file. For a symlink-each
entry, the target directory itself (each expanded child lives in it).

## Acceptance Criteria

- [x] User-scope apply creates missing parents for symlink, symlink-each, copy, template, and edit entries before mise runs.
- [x] System-scope apply emits a `[bootstrap.directories]` entry for the parent of every whole-file target (including symlink-each target directories and template targets) and every edit file.
- [x] `go test ./...` and `go vet ./...` pass.

## Out of Scope

- Onboard: its targets are live paths that already exist.
- Creating parents for non-dotfile targets (mounts already use `[bootstrap.directories]`).
