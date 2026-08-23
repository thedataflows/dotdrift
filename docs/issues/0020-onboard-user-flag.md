---
type: Issue
title: "onboard --user flag; --host --user together"
description: User overlay target flag mirroring --host, combinable with it for a two-layer onboard.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0020: onboard --user flag; --host --user together

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, layers]
- **Assignee**: none
- **Related**: [issue 0019](0019-adopt-alias-and-readme-section.md)
- **Related code**: [`cmd/onboard.go`](../../cmd/onboard.go), [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go)
- **Closing commits**: pending

## Summary

`onboard` gains `--user` (target `users/<username>/modules/<app>`, the
mirror of `--host`), and `--host --user` together onboard into BOTH
overlays in one run: content copied and declared in each layer's
`module.toml`. No flag stays base; a directed (module-file) path still
ignores the flags.

## Details

Run now loops over the target layers (host, user when combined; each
processed with its own copy, adoption sweep, notices with its layer
label, and module.toml merge); the highest-precedence target (user over
host) feeds the single mise config + apply. A missing hostname/username
under the respective flag is an error, mirroring the existing host
guard.

## Acceptance Criteria

- [x] `--user` writes the module under `users/<username>/modules/` and
      nowhere else; notices carry `[users/<username>]`.
- [x] `--host --user` writes BOTH overlay layers with content and
      entries; base untouched.
- [x] `--user` without a resolvable username errors ("username
      required for user overlay").

## Out of Scope

- None.
