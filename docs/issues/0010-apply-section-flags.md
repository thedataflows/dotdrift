---
type: Issue
title: Per-section apply flags
description: "--[no-]packages/tools/dotfiles/mounts/smb/hooks on dotdrift apply - run only the named sections, or all but the negated ones."
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0010: Per-section apply flags

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [apply, cli]
- **Assignee**: none
- **Related**: [cli-surface](../product/cli-surface.md), [issue 0004](0004-resume-cursor-and-status-drift.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), [`cmd/sections.go`](../../cmd/sections.go)
- **Closing commits**: pending

## Summary

`dotdrift apply` gains one negatable flag per module.toml section — `--[no-]packages`, `--[no-]tools`, `--[no-]dotfiles`, `--[no-]mounts`, `--[no-]smb`, `--[no-]hooks` — so a run can execute ONLY the named sections (positives) or everything EXCEPT the negated ones, in any combination.

## Details

`--no-hooks` exists today as a one-off boolean. Generalize: every plan section gets a flag with the same shape.

Semantics (one rule set, evaluated once before the pipeline):

- **No flags** → all sections run (unchanged default).
- **Any positive flags** (`--packages --tools`) → the run executes **exactly** the positive sections. Positives form an allowlist.
- **Negated flags** (`--no-hooks`) → subtract from whatever the base selection is: from all sections when no positive is given, or from the positive allowlist when both forms mix (`--packages --no-tools` = just packages).
- **Empty selection is an error** — `--no-packages --no-tools --no-dotfiles --no-mounts --no-smb --no-hooks` (or an equivalent mix) fails loudly with the valid section names; a silent no-op apply is never OK.
- `DOTDRIFT_NO_HOOKS=1` keeps working: it subtracts `hooks` like `--no-hooks` (the old `--no-hooks` flag spelling keeps working too — it is the negated form of `--hooks`).
- Flags may combine freely with the positional module filter and all other apply flags.

Section→step mapping (`buildSteps` filters by the resolved set):

| Section | Pipeline step(s) |
|---|---|
| `packages` | `packages` |
| `tools` | `tools` |
| `dotfiles` | `dotfiles` (user) + `dotfiles-system` (system-scope entries) |
| `mounts` | `mounts` (services) + mount-destination mkdir |
| `smb` | `smb` (accounts/services + post-actions) |
| `hooks` | `hooks-pre` / `hooks-post` |

Notes:
- The pre-pipeline mise config snapshot and `printPlan` still show the **full** resolved profile (crash-recovery snapshot, issue 0004); the section flags filter which steps execute, not what is planned.
- Resume: a persisted cursor naming a step absent from this run's pipeline is ignored (runs everything selected) — same rule as the module filter changing the step set.
- `mounts` is included although the request's list omitted it: "each section in module.toml" — mounts is one ([profile layout](../product/profile-layout.md)).
- Flag presence (positive vs negated vs absent) is read from kong's parse state (`Flag.Set`), captured via a per-command `AfterApply` hook — a plain bool cannot distinguish `--hooks` from an untouched default. `ApplyCmd.Run`'s signature stays `Run() error`.

## Acceptance Criteria

- [x] Each section flag parses positive and negated (`--packages` / `--no-packages`), on top of kong's `negatable` tag.
- [x] Positives-only run executes exactly those steps; negated-only runs everything else; mixes compose (positive allowlist minus negatives).
- [x] `--no-hooks` and `DOTDRIFT_NO_HOOKS=1` behave as before (hooks subtracted).
- [x] An empty resolved selection errors naming the valid sections.
- [x] Default (no flags) runs the full pipeline — all pre-existing apply tests pass unchanged.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Per-section flags on other commands (`status`, `plan`) — report/plan commands show the whole profile.
- A `--sections=a,b,c` list flag — the per-section booleans cover it with better `--help`.
- Persisting the section selection in resume state — the cursor already tolerates absent steps.

## Notes

- Tests: `cmd/sections_test.go` (resolution truth table, kong parse + negation, kong-context Set detection, unknown-section error), `TestApply_onlyPackages`, `TestApply_onlyToolsAndDotfiles`, `TestApply_sectionFlagsSkipMounts`, `TestApply_noHooksFlag` rewritten over the new resolution; `TestApply_noHooksEnv` unchanged.
