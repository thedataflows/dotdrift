---
type: Issue
title: CI runs fail — non-hermetic service tests and Go 1.26 toolchains behind a Go 1.27 module
description: The service tests read the invoking user and leak an unsettled apply session into the test TempDir, and the lint/e2e/release toolchains are pinned to Go 1.26 while go.mod targets 1.27 — every CI run fails before it reports anything useful.
tags: [bug, ci, tests, toolchain]
timestamp: 2026-09-26T00:00:00Z
---

# ISSUE 0105: CI runs fail — non-hermetic service tests and Go 1.26 toolchains behind a Go 1.27 module

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [ci, tests, toolchain]
- **Assignee**: none
- **Related**: none
- **Related code**: [`internal/service/reads_test.go`](../../internal/service/reads_test.go), [`internal/service/session_behavior_test.go`](../../internal/service/session_behavior_test.go), [`internal/tui/typedinputs_test.go`](../../internal/tui/typedinputs_test.go), [`.github/workflows/`](../../.github/workflows/), [`tests/e2e/`](../../tests/e2e/)
- **Closing commits**: none

## Summary

Every CI run fails, for four independent reasons: (1) `TestService_status_read`
compares the detected facts against a hardcoded username, so it passes only on
the author's machine; (2) `TestSession_previewReportsTTYClassification` starts
an apply session and never settles it, so the session writes into the test
TempDir after the test ends and TempDir cleanup races (a rare, environment-sized
flake); (3) the lint job pins golangci-lint v2.12, whose binary is built with
Go 1.26 and refuses to load a `go 1.27` module; (4) the e2e Dockerfiles and the
release workflow build with Go 1.26, which refuses `go.mod requires go >= 1.27`
under `GOTOOLCHAIN=local`.

## Details

The go.mod language version moved to 1.27 without moving the surrounding
toolchain pins. `actions/setup-go` resolves the test and vet jobs through
`go-version-file` (1.27), so those two steps kept working; everything that
names a version by hand (golangci-lint-action v2.12, the release job's
`go-version: 1.26`, the three `golang:1.26-bookworm` builder images, and
`mise.toml`'s `go = '1.26'`) stayed behind and fails or never loads.

The two test defects are CI-hermeticity, not toolchain: the status read test
passes `nil` facts, so the real `detect.Detect` runs, and then asserts the
detected username equals the fixture's `cri` — true only on the author's
machine. The preview test reads `Preview()` and returns while the session is
still running its steps, so the session's writes land during TempDir cleanup
(`unlinkat: directory not empty`).

A fifth, older defect surfaced once the e2e suite could run at all: 0090
renamed the onboard flag `--app` to the required `--module` and made a
module.toml `app` key a load error, but missed the e2e scenario (`--app
liverc`) and the checked-in demo fixture (`app = "demo"`) — both containers
refused the profile before planning.

## Acceptance Criteria

- [x] `act` (CI workflow) passes the test job on a non-author user (root)
- [x] `act` (CI workflow) passes the lint job — golangci-lint loads the module
- [x] `act` (E2E workflow) builds all three images with Go 1.27
- [x] The service tests pass 60+ repeated `-race` runs without TempDir races

## Out of Scope

- The release job's goreleaser run (tag-gated; the version pin is aligned, the
  run itself needs a real tag push).
- Any behavioral change to the service or the TUI.

## Resolution

Test side: the status read test pins facts through the `Detect` seam and
asserts the read detects once through it (no invoking-user dependency); the
preview test settles the session (`drain` + `Wait`) before the body returns.
Toolchain side: golangci-lint-action `v2.12` → `v2.13` (the v2.13.2 binary is
built with Go 1.27), release `go-version: 1.26` → `1.27`, e2e builder images
`golang:1.26-bookworm` → `golang:1.27-bookworm`, `mise.toml` `go = '1.27'`.
A same-day follow-up moved the linter pin to `v2.14` (mise `2.14` + the
action's `version: v2.14`; v2.14.0 loads the module with 0 issues), so both
pins name the 2.14 line.
The version bump also surfaced one latent `ineffassign` in
`TestTyped_addFormBrowseCommitsIntoField` (an assigned-then-reassigned `f`);
the first binding is now the bare assertion it always was. The e2e scenario's
`--app` becomes `--module` (0090 missed the scenario and the demo fixture's
`app = "demo"` line, which strict loading rejects).
