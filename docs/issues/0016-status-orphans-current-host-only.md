---
type: Issue
title: Status scans orphans only for the current host and user
description: statusModuleLayers derives layer dirs from Selected plus the detected hostname/username, so other hosts'/users' overlays and unselected modules are never checked.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0016: Status scans orphans only for the current host and user

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [status, drift, orphans]
- **Assignee**: none
- **Related**: [issue 0012](0012-status-orphans-section.md), [issue 0014](0014-dir-source-subtree-orphan-false-positive.md)
- **Related code**: [`cmd/status.go`](../../cmd/status.go), [`internal/drift/orphans.go`](../../internal/drift/orphans.go)
- **Closing commits**: pending

## Summary

`statusModuleLayers` builds the orphan-scan layer list from `p.Selected`
plus the CURRENT detected hostname/username. Orphans are profile-content
drift, not live-system drift: leftovers under `hosts/<other>/modules/*`
or `users/<other>/modules/*` never show on this machine, and a module
not selected here (when-filter) is never scanned at all.

## Details

The scan must cover **every module layer directory in the profile**:
`modules/*`, `hosts/*/modules/*`, `users/*/modules/*`, regardless of the
current facts. References must then come from the layer **declarations**
(a resolved plan only covers the current host's view): a directory
source references its whole subtree anchored to the declaring layer
(issue 0014 rule), a file source references the file each host/user view
resolves it to (user > host > base, first existing — so a base copy
shadowed on every host is an orphan, but one that still deploys on a
host without an overlay copy stays referenced). This keeps the shipped
single-host semantics (shadowed base copy = orphan) while making every
layer checkable from any machine.

## Acceptance Criteria

- [x] Orphans under any `hosts/<h>/modules/*` and `users/<u>/modules/*`
      show in `status` run on a different machine, under their own
      layer-root heading.
- [x] A module that exists only in another host's layer (not selected
      here) is scanned; its declared sources are not flagged, its extra
      files are.
- [x] A base file-source copy shadowed on every host is an orphan; a
      copy that still resolves for some host is not.

## Out of Scope

- Cross-host plan resolution for the live-system sections (packages,
  tools, dotfiles, mounts, smb) — those stay current-host views.

## Notes

`CheckOrphans` drops its plan parameter; `ReferencedPaths` is exported
so onboard computes adoption references with identical semantics.
