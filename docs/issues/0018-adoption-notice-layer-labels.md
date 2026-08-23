---
type: Issue
title: Adoption notices spell layer labels like status headings
description: "adopted:/would adopt: suffix [hosts/<hostname>] / [users/<username>] instead of [host]/[user], matching the status orphans group headings."
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0018: Adoption notices spell layer labels like status headings

- **Type**: chore
- **Status**: done
- **Priority**: low
- **Labels**: [onboard, display]
- **Assignee**: none
- **Related**: [issue 0017](0017-onboard-adoption-units-and-profile-paths.md)
- **Related code**: [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go)
- **Closing commits**: pending

## Summary

Onboard adoption notices (`adopted:` / `would adopt:`) now suffix the
layer as `[base]`, `[hosts/<hostname>]`, or `[users/<username>]` — the
same headings the status orphans section groups by — instead of the bare
`[host]`/`[user]`.

## Details

`layerLevel` became `layerLabel`; the directed-host owner derivation
reads the label prefix. Docs spellings synced (profile-layout,
cli-surface, contract invariant 5).

## Acceptance Criteria

- [x] Host and user adoption notices carry `[hosts/<hostname>]` /
      `[users/<username>]`; base stays `[base]`.

## Out of Scope

- None.
