---
type: Issue
title: Apply the ponytail over-engineering audit — delete the dead code, one lipgloss
description: Whole-repo audit applied biggest-cut-first: the generate wizard cluster (machines, decisions, the equivalence harness), the smb.Step machinery the service layer replaced, dead helpers, the folded back() esc discipline, test-only production code, and the lipgloss v1 dependency. Two deliberate keepers, one bug found and routed to 0084.
tags: [task, audit, cleanup, tui, service]
timestamp: 2026-09-21T18:30:00Z
---

# ISSUE 0083: Apply the ponytail over-engineering audit

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [audit, cleanup]
- **Assignee**: none
- **Related**: [ADR-0007](../adr/0007-generate-cli-only.md), [contract invariant 15](../product/contract.md), [0084](0084-restore-dialog-plan-never-lands.md)
- **Related code**: [`internal/generate/`](../../internal/generate/), [`internal/smb/`](../../internal/smb/), [`internal/tui/`](../../internal/tui/), [`internal/executil/`](../../internal/executil/), [`main.go`](../../main.go)
- **Closing commits**: 69903d7, 0e6ff9f, 50f5de3, 82f03c3, e99bcef

## Summary

The repo-wide over-engineering audit was applied biggest-cut-first:
five commits deleted everything dead or duplicated, with each finding
verified by `golang.org/x/tools/cmd/deadcode` (run with and without
test roots) plus a manual caller check before the cut. Net on code:
**-726 production lines, -845 test lines** (vendor lost ~15,000 more).
Dependencies: **-1 direct, -2 indirect**.

## Details

Findings and what happened to each:

- **generate wizard cluster** (`delete:`): `machines.go` (301 lines),
  `decisions.go` (48) and their tests (256), plus
  `cmd/generate_tui_equivalence_test.go` (264). The
  `MountsWizard`/`SmbWizard` machines and the flow-decision helpers had
  zero production callers — the CLI and the tui editors both assemble
  through `MountsInput`/`SmbInput` directly. ADR-0007 planned the
  equivalence harness to retarget at editor prefill when the editors
  landed (0065-D9); they never did, and the editors prefill through the
  same builders, making the harness a wrapper round-trip.
  `generate.ModuleDir`, a wrapper kept "for the generate wizard", died
  with the cluster (callers use `moduleDir`). Contract 15 and
  cli-surface now point at the live seam.
- **smb.Step machinery** (`delete:`): the `Step` type, its nine methods,
  and the `var _ apply.Step` assertion (~161 lines), plus ~340 lines of
  Step tests. `smbBootstrapStep` (service/steps.go) replaced the step —
  declarative convergence moved to mise bootstrap, the interactive
  parts live in `PostBootstrap`, which keeps real coverage in
  `cmd/apply_test.go`. The `geteuid`/`isTTY` test-seam vars lost their
  only substitutors and are inlined as direct calls.
- **dead helpers** (`delete:`): `service.New`, `profile.ModuleDir`,
  `tomlsplice.Families` (no callers anywhere), and the dialog
  interface's `back() bool` with its five identical implementations —
  the 0072 esc discipline folded into the compositor's pop rule
  (modals.go) when dialogs became modals; the methods never lost their
  interface slot. `TestManage_backDiscipline` drove the dead method and
  dies with it; the pop rule itself is covered by compositor tests.
- **test-only production code** (`delete:`/relocate): `OrStdout`,
  `OrStderr`, `BoldSeq`, and `RenderStatusSummary` had only test
  callers (the summary's test now composes header + note, the same
  bytes). `SwapSudoRunner` died with `sudo_test.go` converted to an
  in-package test that stubs the seam var directly. The seven TUI
  assertion seams (`wsDraftFor`, `visibleLabels`, `actionLabels`,
  `placeholderOrBody`, `atSection`, `draftEdits`, `draftErrs`) are the
  suite's vocabulary — 80+ call sites — so they moved to
  `helpers_test.go` instead of dying: production files carry production
  code only.
- **lipgloss v1** (`native:`): `main.go` was the only importer of
  `github.com/charmbracelet/lipgloss` (the TUI is on v2), kept for one
  bold-red fatal error line. The v2 style over a colorprofile writer
  renders the same thing. `go mod tidy` drops lipgloss v1 and its
  transitives `go-osc52/v2` and `x/cellbuf`; vendor syncs.

**Kept deliberately**, with reasons: the `mise.FakeRunner` methods
deadcode flags are interface-compliance stubs; `godotenv` backs the
documented, tested `.env` auto-load (log history, `DOTDRIFT_NO_ENV`);
`go-difflib` is the load-bearing drift diff (no stdlib equivalent);
`testify` spans 138 test files — the churn dwarfs the value.

**Found during the pass, out of audit scope** (correctness, not
over-engineering): `restorePlanMsg` has no production consumer, so the
restore dialog's plan resolution never lands in the shipped TUI. Filed
as [0084](0084-restore-dialog-plan-never-lands.md); `applyPlan` stayed
untouched here.

## Acceptance Criteria

- [x] No production caller for any deleted symbol (deadcode run with `-test` confirms; final list holds only the two documented keepers)
- [x] `go test ./... -count=1` green after every commit (20 packages, 0 FAIL)
- [x] `go vet ./...` and golangci-lint clean on the final tree
- [x] Docs that named deleted code amended in the same commit (contract 15, cli-surface, service-api)
- [x] The correctness find routed to its own issue, not fixed inside the audit

## Out of Scope

- The `restorePlanMsg` bug (0084) — audit scope is complexity only.
- Replacing testify or go-difflib (evaluated, declined — see Details).
- Vendor-tree strategy (vendoring itself was not an audit finding).

## Notes

The audit report was chat-delivered and re-derived for this pass; the
fresh scan reproduced the delivered headline (biggest cut: the wizard
cluster; `-2 deps` — landed as one direct plus two indirect module
lines).
