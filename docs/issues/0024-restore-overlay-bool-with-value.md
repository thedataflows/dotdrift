---
type: Issue
title: Restore bool-like --host/--user (bare flags must compose)
description: "Plain string flags error on `--host --user` (kong demands a value); bring back the overlayFlag BoolMapperValue."
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0024: Restore bool-like --host/--user (bare flags must compose)

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, cli, layers]
- **Assignee**: none
- **Related**: [issue 0021](0021-onboard-overlay-flag-values.md), [issue 0023](0023-overlay-flag-empty-value-means-current.md)
- **Related code**: [`cmd/overlay_flag.go`](../../cmd/overlay_flag.go), [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: pending

## Summary

Kong's plain string flags require a value: `--host --user` fails with
`--host: expected string value`. The `overlayFlag` bool-like type
(kong `BoolMapperValue`) is restored — it accepts the bare spelling
(= current host/user), the `=value` spelling (explicit), and the omitted
one (base layer), and bare flags compose freely.

## Details

Same semantics as issue 0021 (`Set`/`Value` + `overlayOwner`:
explicit wins, bare falls back to detection). Regression test pins
`--host --user` parsing (the exact failure that motivated the restore)
alongside the full spelling matrix, including `--host=` (empty value =
current, subsuming 0023's pointer semantics) and the path-intact guard.

## Acceptance Criteria

- [x] `--host --user` parses (no "expected string value" error).
- [x] Bare = current host/user; `=value` = explicit; omitted = base;
      `--host=` = current.
- [x] A bare flag never consumes the next token.

## Out of Scope

- Space-separated values (`--host myhost`): binds only via `=`.
