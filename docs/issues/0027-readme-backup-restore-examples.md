---
type: Issue
title: "README: backup/restore usage examples"
description: "Add a Backups and restoring section with worked examples (apply --backup, restore --list/--dry-run/--gen, re-onboard to keep). Also aligned backup/list output to profile-relative paths."
tags: [issue, docs]
timestamp: 2026-08-24T00:00:00Z
---

# ISSUE 0027: README: backup/restore usage examples

- **Type**: docs
- **Status**: done
- **Priority**: low
- **Labels**: [readme, backup, restore]
- **Related**: [issue 0025](0025-apply-backup-copy-mode-destinations.md), [issue 0026](0026-restore-command.md)
- **Closing commits**: pending

## Summary

README gained a "Backups and restoring" section with worked examples:
back up before an apply (tree layout), browse with `restore --list`,
restore newest / `--dry-run` / `--gen`, and re-onboard to keep restored
content. While writing the examples, two outputs were found printing
absolute profile paths (`apply --backup` summary, `restore --list` full
view) while restore notices print profile-relative — both now print
profile-relative consistently, pinned by tests.

## Details

Output shapes in the examples are the test-pinned shapes, not
hand-written approximations: `backup: N path(s) -> <module>/backups/<gen>`,
list headings `modules/<m>:` with `  <gen>  N file(s)`, filtered lines
`  modules/<m> <gen>`, notices `restored: <target> (<module>/backups/<gen>)`.

## Verification

- `TestApply_backupSnapshotsCopyTargets`: summary lines extracted and
  asserted relative (previously only the `backup:` prefix was checked).
- `TestRestore_list`: full-view headings asserted profile-relative.
