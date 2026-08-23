---
type: Issue
title: "apply --backup: snapshot copy-mode destinations before overwrite"
description: "Copy is the only mode whose apply overwrites destination content; add an optional pre-apply backup into the declaring module's backups/ tree."
tags: [issue, product]
timestamp: 2026-08-24T00:00:00Z
---

# ISSUE 0025: apply --backup: snapshot copy-mode destinations before overwrite

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [apply, dotfiles, copy-mode, backup]
- **Assignee**: none
- **Related code**: [`internal/backup/backup.go`](../../internal/backup/backup.go), [`cmd/apply.go`](../../cmd/apply.go), [`internal/drift/orphans.go`](../../internal/drift/orphans.go)
- **Closing commits**: pending

## Summary

`dotdrift apply --backup` snapshots every existing copy-mode destination
into the profile before the pipeline runs. Backups land next to the
profile content that replaces them: inside the module layer directory
that declared the entry, under `backups/<generation>/`, mirroring the
absolute target path (`modules/easyeffects/backups/20260824-153000/home/cri/.config/easyeffects/db/easyeffectsrc`).

## Details

- **Why copy mode only**: a symlink destination is recreated as a link
  and edit entries are marker-scoped; a copy overwrite is the one apply
  operation that silently loses destination content.
- **What is backed up**: every existing target of a whole-file copy-mode
  entry in the resolved plan (all existing, not only differing — a
  safety net has no diff logic to get wrong). Directory targets are
  copied recursively; symlinks are followed (the resolved content is
  what mise would overwrite); missing targets are skipped.
- **Layer-aware location**: an entry won by `hosts/<h>` backs up into
  `hosts/<h>/modules/<m>/backups/`, a user winner into
  `users/<u>/modules/<m>/backups/` — each backup sits next to the
  module.toml that declared the entry.
- **One generation per run**: every module shares the run's timestamp
  directory, so a generation is coherent across modules.
- **Fail closed**: an unreadable target aborts the apply — a safety flag
  must not fail open.
- **Skipped when `--no-dotfiles`**: no copy step runs, so there is
  nothing to safeguard.
- **Runtime output, not content**: a `backups/` directory directly under
  a module layer root is skipped by the status orphan scan (issue 0016)
  and cannot be adopted by onboard (its paths do not invert to targets);
  deeper `backups` dirs are still profile content. Users may want to
  gitignore `backups/` in the profile repo.
- **ponytail: no retention/pruning** — generations accumulate until
  cleaned by hand; upgrade path is a configurable cap if real profiles
  ever need it.

## Verification

- `internal/backup`: mirror layout, mode preservation, recursive dir
  copy, symlink follow, unreadable-target failure.
- `cmd`: end-to-end wiring (base + host-layer modules, symlink target
  untouched, summary line, no backups by default, skip with dotfiles
  deselected).
- `internal/drift`: orphan scan skips module-level `backups/`.
- `internal/onboard`: adoption never touches `backups/` (pinned).
