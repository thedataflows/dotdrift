---
type: Issue
title: "TICKET: package version pins"
description: Decision ticket — schema for pinning package versions in module.toml (apt name=version, dnf) and how status reports version mismatch.
tags: [issue, wayfinder:grilling, mise, packages]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0049: TICKET — package version pins

- **Type**: feature
- **Status**: open
- **Priority**: low
- **Labels**: [wayfinder:grilling, mise, packages]
- **Assignee**: none
- **Related**: [map 0047](0047-mise-bootstrap-adoptions-map.md), [alignment A5](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/profile/profile.go`](../../internal/profile/profile.go), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: none

## Question

Should `packages.present` support version pins, and in what spelling?

Open decisions:

1. **Spelling** — inline in the string (`"apt:curl=8.5.0-2ubuntu10"` — but
   `=` collides with nothing today; bare names are manager-less) vs a table
   form (`{ name = "curl", version = "..." }`) vs a parallel map. Today
   `present` is a plain string list; a table form complicates the layer-merge
   (cancel-by-name must still work for `absent`).
2. **Backend coverage** — mise pins only apt (`name=version`) and dnf;
   pacman/aur skip pins with a warning (rolling release). Is a pin on an
   unsupported backend a dotdrift-side resolve error (fail loud) or passed
   through to mise's warning?
3. **Status** — dotdrift's package probe is installed/not-installed; does
   pinned status need `version mismatch` reporting (probe installed version
   via the backend), or is that deferred to the standing
   status-engine-convergence question?
4. **Merge** — same package pinned differently in two layers: nearer layer
   wins (consistent with everything else), presumably; conflict across
   *modules* is today's present/absent machinery — does a version difference
   count as a conflict?

Demand check: no package in the dogfood profile is pinned today. This may be
a candidate for ruling out (YAGNI) rather than implementing.
