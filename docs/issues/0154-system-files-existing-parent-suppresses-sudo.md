---
type: Issue
title: System files under an existing parent lose their sudo prompt
description: Declaring an existing target parent in [bootstrap.directories] makes mise plan the files under it as user-level, so a copy to /root fails with Permission denied instead of escalating.
tags: [issue, dotfiles, bootstrap, scope]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0154: System files under an existing parent lose sudo

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: []
- **Assignee**: none
- **Related**: [0151](0151-dotfile-operations-create-parent-directories.md)
- **Related code**: [`internal/service/`](../../internal/service/)
- **Closing commits**: none

## Summary

Regression from issue 0151. A system-scope copy to /root/.bashrc now fails with 'mise ERROR failed copy: ... Permission denied (os error 13)' and no sudo prompt. Before 0151, mise escalated the change in its privileged batch and prompted.

## Details

Issue 0151 emits the parent of every system dotfile target as a [bootstrap.directories] entry in the same config as [bootstrap.files]. Verified against mise 2026.11.8:

- [bootstrap.files] alone, target under a non-writable directory: mise plans a privileged change and escalates (on a terminal it prompts for sudo; without one it fails with the exact privileged command).
- The same file plus a [bootstrap.directories] entry for its **existing** parent: mise plans the file as a user-level change and copies as the current user. /root exists, so the directory needs no change, but the file under it drops out of the privileged batch.
- A [bootstrap.directories] entry for a **missing** parent: mise escalates when creating it needs privilege, and the file rides along. Creating missing parents this way is sound.

So a parent directory must be declared only when it does not exist. An existing parent is left undeclared, and mise classifies each file by its own target again — the pre-0151 behavior.

## Acceptance Criteria

- [x] A system dotfile parent that already exists is not emitted as a [bootstrap.directories] entry.
- [x] A system dotfile parent that does not exist is still emitted, for whole-file entries (including symlink-each target directories) and edit files, so issue 0151's create-parents guarantee holds.
- [x] Mount destination directories are still always emitted.
- [x] An edits-only system step whose edit parents all exist runs no bootstrap pass.
- [x] Tests pin the existing-parent and missing-parent cases; docs and contract state the rule.

## Out of Scope

- Classifying whole-file system steps for the TUI sudo gate. Pre-0151, whole-file elevation was mise's own prompt-or-fail-loud behavior (issue 0042), and this fix restores exactly that.

## Notes

Verified live with mise 2026.11.8 in a scratch tree: a files-only config under a mode-0555 directory escalates; adding the existing parent to [bootstrap.directories] makes mise apply as the current user; an unrelated directory entry changes nothing; a missing parent entry escalates and creates.
