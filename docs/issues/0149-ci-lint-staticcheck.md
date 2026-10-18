---
type: Issue
title: "0149: CI lint job fails on 9 staticcheck findings"
description: The CI lint job (act, golangci-lint v2.14) reports 9 staticcheck findings — 2 QF1001 De Morgan forms and 7 SA4006 unused test assignments — and fails before the gosec step runs.
tags: [issue, chore, ci]
timestamp: 2026-10-19T00:00:00Z
---

# ISSUE 0149: CI lint job fails on 9 staticcheck findings

- **Type**: chore
- **Status**: done
- **Priority**: medium
- **Labels**: [ci]
- **Assignee**: none
- **Related**: [0136](0136-mise-interactive-foreground.md)
- **Related code**: [`internal/mise/mise.go`](../../internal/mise/mise.go), [`internal/tui/`](../../internal/tui/)
- **Closing commits**: `1c94443`

## Summary

`act -j lint` (the CI lint job: golangci-lint v2.14, then gosec) fails
in the golangci-lint step with 9 staticcheck findings, so the gosec
step never runs:

- QF1001 (De Morgan): `internal/mise/mise.go:236` (the 0136 terminal
  check), `internal/tui/keymap_test.go:173`.
- SA4006 (value never used): `internal/tui/dialog_test.go:597`,
  `internal/tui/modals_test.go:348`,
  `internal/tui/statusview_test.go:194,216,229,286,436` — test steps
  whose returned compositor is never read again.

## Fix

Apply De Morgan at the two QF1001 sites (same condition, no behavior
change) and drop the dead compositor assignments at the SA4006 sites
(keep the returned `cmd` where a test asserts on it). No production
behavior changes.

## Acceptance Criteria

- [x] `act -j lint` passes end to end, including the gosec step.
- [x] `go test ./...` and `go vet ./...` pass.
