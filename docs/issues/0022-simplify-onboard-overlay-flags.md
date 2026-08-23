---
type: Issue
title: Simplify onboard --host/--user to plain string flags
description: "Drop the overlayFlag BoolMapperValue type; plain strings, empty = base layer."
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0022: Simplify onboard --host/--user to plain string flags

- **Type**: chore
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, cli]
- **Assignee**: none
- **Related**: [issue 0021](0021-onboard-overlay-flag-values.md)
- **Related code**: [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: pending

## Summary

The `overlayFlag` custom type (kong `BoolMapperValue`, Decode + IsBool +
an owner resolver) is replaced by plain string flags with an empty-check:
`--host=<hostname>` / `--user=<username>` select that layer, empty or
omitted means base. Explicit values still compose (`--host` + `--user`
onboards into both layers).

## Details

Empty flag = not selecting that overlay; the hostname/username still
fall back to the detected facts for the adoption claim context. Bare
`--host` no longer parses (kong string flags need a value; use `=`),
which also removes the value-binding special case entirely.

## Acceptance Criteria

- [x] `--host=<name>` / `--user=<name>` / both / omitted behave as
      documented, via plain strings and one empty-check.
- [x] `cmd/overlay_flag.go` deleted.

## Out of Scope

- None.
