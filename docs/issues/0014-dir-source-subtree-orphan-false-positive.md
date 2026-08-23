---
type: Issue
title: Directory sources flag their own subtree as orphans
description: A symlink/copy entry whose source is a directory deploys the whole subtree, but status flags every file under it as an orphan.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0014: Directory sources flag their own subtree as orphans

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [status, drift, orphans]
- **Assignee**: none
- **Related**: [issue 0012](0012-status-orphans-section.md)
- **Related code**: [`internal/drift/orphans.go`](../../internal/drift/orphans.go)
- **Closing commits**: pending

## Summary

`referencedSources` marks only the exact `source` path of a whole-file
entry. When the source is a **directory** (`mode = "symlink"` over
`home/.config/app`, the exact shape `onboard` produces by default), mise
deploys the entire subtree, yet status reports every file under it as an
orphan.

## Details

Issue 0012 fixed this class of false positive for `symlink-each` only:
its source subtree is walked and attributed to the declaring layer. A
plain `symlink` (or `copy`) entry pointing at a directory deploys the
same subtree through a single link (or recursive copy), so the same
referencing rule applies. The general rule: **an entry's reference set
is its source file, or the whole subtree of its source directory,
anchored to the layer whose module.toml declares the entry.**

This blocks issue 0015: onboard adoption must not adopt files a
directory entry already deploys.

## Acceptance Criteria

- [x] `status` reports no orphans for files under a directory source of
      any whole-file mode (`symlink`, `copy`), at any depth.
- [x] The subtree anchors to the declaring layer, identical to
      symlink-each semantics (overlay files at the same rel-path that
      nothing references stay orphans).
- [x] Existing symlink-each behavior is unchanged.

## Out of Scope

- Merging overlay trees into one deployed subtree (shadowing stays
  whole-entry).

## Notes

Generalizes `symlinkEachSourceTrees` to a dir-source walk; the
symlink-each branch collapses into it.
