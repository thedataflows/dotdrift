---
type: Issue
title: Status misses invalid symlink sources
description: symlink/symlink-each entries whose profile-side source no longer exists reported OK.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0011: Status misses invalid symlink sources

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [status, drift, dotfiles]
- **Assignee**: none
- **Related**: [issue 0012](0012-status-orphans-section.md), [cli-surface](../product/cli-surface.md)
- **Related code**: [`internal/drift/`](../../internal/drift/), [`cmd/status.go`](../../cmd/status.go)
- **Closing commits**: pending

## Summary

`dotdrift status` validated that each managed symlink points at the right path but never checked that the path it points at (the profile-side source file) still exists. A deleted source left the check reporting OK for a dangling link.

## Details

Two gaps, both fixed in `internal/drift`:

1. **`symlink` (and per-child `symlink-each`) — dangling link**: the probe compared `readlink(target)` to the expected source path and stopped. Now, when the link matches, the source path is statted: missing → Drift `source missing (dangling link)`; a stat error → Unknown. A healthy link stays OK.
2. **`symlink-each` — stale children**: expansion lists the *live* source directory, so a source file deleted after apply disappeared from the probe set entirely while its leftover target symlink kept dangling forever. The check now also scans the **target directory**: every symlink that points *into this entry's source dir* is validated — its source missing → Drift `stale link: source removed`. Links pointing elsewhere (user files, other modules) are ignored, as are non-symlinks.

New probes on `drift.Probes` (both defaulted in `DefaultProbes`, nil-guarded): `Stat func(path string) (bool, error)` (exists, file or dir; not-exist is `(false, nil)`) and `ListDir func(path string) ([]string, error)` (direct child names). `ListDir` is elevated like the other file probes (`sudo ls -1`) for root-owned system targets; `Stat` only ever touches profile-side sources and needs no elevation.

The stale scan runs eagerly at probe-build time (same precedent as `ResolveBootstrapFiles`' directory listing) and emits one finding per stale child — a clean target directory adds nothing, so existing output is unchanged.

## Acceptance Criteria

- [x] symlink entry whose source file was deleted reports Drift `source missing (dangling link)`, not OK.
- [x] symlink-each leftover target link (source removed) reports Drift `stale link: source removed`; live children stay OK.
- [x] Foreign links (outside the entry's source dir) and plain files in the target dir are not flagged.
- [x] Clean runs produce no new findings (pre-existing tests unchanged).
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Auto-removal of stale/dangling links — status is a read-only report; apply owns convergence.
- copy/template modes — copy already reads the source (errors surface as Unknown); template is existence-only by documented contract.

## Notes

- Tests: `internal/drift/sources_test.go` — `TestCheck_dotfilesSymlinkSourceMissing`, `_SourcePresentStaysOK`, `_SymlinkEachStaleChild`, `_StaleChildForeignLinkIgnored`, `_sourceProbesNilGuard`.
