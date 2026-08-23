---
type: Issue
title: "onboard --host/--user take an optional value"
description: Bare flag keeps the current host/user; --host=<hostname> / --user=<username> set an explicit one.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0021: onboard --host/--user take an optional value

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, layers, cli]
- **Assignee**: none
- **Related**: [issue 0020](0020-onboard-user-flag.md)
- **Related code**: [`cmd/overlay_flag.go`](../../cmd/overlay_flag.go), [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: pending

## Summary

`--host` and `--user` accept an optional value: bare keeps the current
behavior (detected host/user), `--host=<hostname>` / `--user=<username>`
target an explicit layer.

## Details

Kong has no native optional-value flag, but a field type implementing
`BoolMapperValue` (Decode + IsBool) is exactly that: bare `--host` is
valid (no value consumed) and `--host=x` binds through the `=` form.
Because bool-like flags only take values via `=`, a bare `--host` before
a positional path can never swallow the path (the failure mode plain
string flags have). `overlayOwner` resolves flag -> owner (explicit
wins, bare falls back to the detected fact). A plain-string `optional`
tag was probed and rejected: kong applies it to arguments, not flag
values — bare string flags always error on EOL.

## Acceptance Criteria

- [x] `--host` bare selects the detected host; `--host=<name>` selects
      the explicit one (same for `--user`).
- [x] A bare flag never consumes the next token (paths stay intact).
- [x] Explicit values work alone and combined (`--host=h --user=u`).

## Out of Scope

- Space-separated values (`--host myhost`): bool-like semantics bind
  only via `=`, deliberately (path-eating hazard).
