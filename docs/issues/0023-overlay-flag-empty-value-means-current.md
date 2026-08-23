---
type: Issue
title: "onboard --host=/--user= (empty value) means the current host/user"
description: The plain-string rewrite treated --host= as base layer; empty value must select the CURRENT host/user layer, omitted stays base.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0023: onboard --host=/--user= (empty value) means the current host/user

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [onboard, cli, layers]
- **Assignee**: none
- **Related**: [issue 0022](0022-simplify-onboard-overlay-flags.md)
- **Related code**: [`cmd/onboard.go`](../../cmd/onboard.go)
- **Closing commits**: pending

## Summary

The plain-string simplification (0022) documented "empty = base layer",
conflating `--host=` with an omitted flag: both are the zero string, so
`--host=` (empty value) silently onboarded into base instead of the
current host's layer.

## Details

The flags are `*string` now: nil = omitted (base layer), non-nil with an
empty value (`--host=`) = the DETECTED host/user, non-nil with a value =
that explicit layer. Two plain checks in `cmd/onboard.go`; kong's
pointer support distinguishes `--host=` from absent without any custom
mapper. Explicit values still compose (`--host` + `--user` = both
layers).

## Acceptance Criteria

- [x] `--host=` onboards into `hosts/<detected-host>/`, not base.
- [x] `--user=` onboards into `users/<detected-user>/`.
- [x] Omitted flags keep the base layer; explicit values keep winning.

## Out of Scope

- Bare `--host` (no `=`): kong string flags require a value.
