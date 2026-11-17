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

A [dotfiles] source that starts with / is an absolute path. It must be used as-is. Before this fix it was joined to the module directory, so resolve failed with 'not found in any layer' for a file that exists.

## Details

resolveSource in internal/resolve/resolve.go does filepath.Join(layer.path, rel) for every source. In Go, Join('/a', '/b') gives /a/b, so an absolute source becomes a path inside the module directory that does not exist.

An absolute source names one file or directory on the machine. It does not take part in the layer search: the same path is the source no matter which layer declares the entry. This holds for every mode that reads a source: symlink, symlink-each, copy, template, and mode = 'edit' — all of them go through resolveSource.

The orphan scan in internal/drift/orphans.go joins the declared source to the layer directory in the same way. An absolute source must not mark a file inside the module as referenced.

Amendment (2026-11-17): an absolute source is not existence-checked at resolve time. It is returned as-is whether or not the file exists; a missing one fails in mise at apply time. Exception: mode = 'edit' still reads the source at resolve time to inline its contents as a block, so a missing edit source fails at that read.

## Acceptance Criteria

- [x] A dotfile entry with an absolute source resolves to that exact path when the file exists, for whole-file modes and for mode = 'edit'.
- [x] A missing absolute source still resolves to that exact path (amended 2026-11-17: no upfront existence check).
- [x] A relative source still resolves through the layers as before, and a source that escapes the module directory is still rejected.
- [x] The orphan scan does not treat an absolute source as a path inside the module directory.
- [x] Docs state that a source starting with / is absolute and is used as-is.

## Out of Scope

- ~/ expansion in a source. Only a leading / makes a source absolute.
- Onboard: it writes module-relative sources itself and does not read declared sources.

## Notes

Downstream code already handles an absolute resolved source: ResolveBootstrapFiles and the drift checks test filepath.IsAbs before joining. The fix belongs in resolve, plus the join sites in the orphan scan.
