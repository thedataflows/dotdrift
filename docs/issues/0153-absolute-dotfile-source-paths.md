---
type: Issue
title: Absolute dotfile source paths are used as-is
description: A dotfile source that starts with / is an absolute path and must not be joined to the module directory.
tags: [issue, dotfiles, resolve]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0153: Absolute dotfile source paths

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: []
- **Assignee**: none
- **Related**: none
- **Related code**: [`internal/resolve/`](../../internal/resolve/), [`internal/drift/`](../../internal/drift/)
- **Closing commits**: none

## Summary

A `[dotfiles]`NaN`resolveSource` in `internal/resolve/resolve.go` does `filepath.Join(layer.path, rel)` for every source. In Go, `Join("/a", "/b")` gives `/a/b`, so an absolute source becomes a path inside the module directory that does not exist.

An absolute source names one file or directory on the machine. It does not take part in the layer search: the same path is the source no matter which layer declares the entry. This holds for every mode that reads a source: symlink, symlink-each, copy, template, and `mode = "edit"` — all of them go through `resolveSource`.

The orphan scan in `internal/drift/orphans.go` joins the declared source to the layer directory in the same way. An absolute source must not mark a file inside the module as referenced.

## Acceptance Criteria

- [x] A dotfile entry with an absolute source resolves to that exact path when the file exists, for whole-file modes and for `mode = "edit"`NaN`~/`NaN`ResolveBootstrapFiles` and the drift checks test `filepath.IsAbs` before joining. The fix belongs in resolve, plus the two join sites in the orphan scan.
