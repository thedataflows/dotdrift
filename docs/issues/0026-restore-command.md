---
type: Issue
title: "restore command: copy backed-up copy-mode targets back to their live paths"
description: "apply --backup (issue 0025) writes snapshots with no restore path; add `dotdrift restore` as the inverse."
tags: [issue, product]
timestamp: 2026-08-24T00:00:00Z
---

# ISSUE 0026: restore command: copy backed-up copy-mode targets back to their live paths

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [restore, backup, cli, copy-mode]
- **Assignee**: none
- **Related**: [issue 0025](0025-apply-backup-copy-mode-destinations.md)
- **Related code**: [`internal/backup/restore.go`](../../internal/backup/restore.go), [`cmd/restore.go`](../../cmd/restore.go)
- **Closing commits**: pending

## Summary

`dotdrift restore <targets...>` is the inverse of `apply --backup`: it
copies backed-up copy-mode destinations back to their live target paths.
Bare `restore` is an error — at least one target is required. `--list`
browses generations (all, or the ones holding a target), `--gen <ts>`
pins a generation (default: the newest holding each target), `--dry-run`
previews.

## Details

- **Discovery** scans every module layer directory on disk (same walk as
  the status orphan scan): backups are recovery data, not current-view
  content, and a target may be held by `hosts/<h>` or `users/<u>` layers.
- **Mapping** rides the mirror rule: the path under a generation
  directory IS the absolute target minus the leading separator.
- **Mechanics** beyond a plain `cp`: missing parent directories are
  created; a symlink sitting at the target is removed, not followed
  (writing through it would clobber the profile source after a mode
  switch); file modes come from the backup.
- **Elevation**: a target the current user cannot write (system files)
  restores via `sudo install -D -m <mode>` — install lands the file
  root-owned (cp -p would stamp the backup's user owner onto `/etc`) and
  creates leading directories. Sudo's timestamp cache means at most one
  password prompt per run.
- **Ambiguity is an error**: the same target in two modules' backups
  fails naming both (cross-module target conflicts are resolve errors,
  so this means stale backups) — a wrong-file silent restore is
  unacceptable in a recovery tool. A `--gen` holding nothing for the
  target names the generations that do hold it.
- **Drift hint**: after restoring, the command prints that apply will
  overwrite the targets again — re-onboarding is the path to keeping
  restored content. Restore itself involves no mise and touches no state.
- **ponytail: empty backed-up directories are not represented** — the
  index is per file, so a directory that was empty at backup time is not
  recreated; upgrade path is index-by-unit if that ever bites.

## Verification

- `internal/backup`: `Generations` newest-first; `RestoreItem`
  roundtrip (Run → mutate → restore), symlink replacement (link
  destination untouched), missing parents, missing/relative-target
  errors.
- `cmd`: end-to-end newest-generation restore (content + mode + notice
  + hint), `--gen` pin, `--dry-run` touches nothing, missing-target and
  `--gen`-mismatch errors, ambiguous-target error, symlinked live
  target, elevated seam (mode forwarded), `--list` both views,
  bare-requires-target, kong parse.
