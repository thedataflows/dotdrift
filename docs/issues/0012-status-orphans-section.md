---
type: Issue
title: Status orphans section
description: Show unreferenced module files per host/user/module in a new orphans section.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0012: Status orphans section

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [status, drift, profile]
- **Assignee**: none
- **Related**: [issue 0011](0011-status-symlink-source-validity.md), [profile layout](../product/profile-layout.md), [cli-surface](../product/cli-surface.md)
- **Related code**: [`internal/drift/orphans.go`](../../internal/drift/orphans.go), [`cmd/status.go`](../../cmd/status.go)
- **Closing commits**: pending

## Summary

`dotdrift status` ignored files inside module directories that no `[dotfiles]` entry manages — leftovers from edits, renames, or never-wired content. A new `orphans` report section lists them, attributed per module and layer (`<module> [base|host|user]`).

## Details

An **orphan** is a file inside a selected module's layer directory that the resolved plan does not reference:

- not the `source` of any whole-file entry (symlink/copy/template),
- not the `source` of a template edit entry,
- not a **direct child** of a `symlink-each` source directory (implicitly deployed), and
- not `module.toml` (the manifest itself).

Everything else in the walk is reported: `orphans:` section, item = path relative to the layer module directory, detail `not referenced by [dotfiles]`, module attribution `<dir> [<layer>]` (e.g. `shell [host]`) — so host- and user-overlay leftovers are distinguishable from base ones.

Deliberate semantics:

- The referenced set comes from the **resolved plan** (what apply actually uses). A file in a lower layer shadowed by a higher layer's same-named source is an orphan — apply never reads it; the report makes the dead weight visible.
- Nested files under a `symlink-each` source directory (depth ≥ 2) are orphans: `symlink-each` deploys direct children only (documented in [profile layout]), so deeper files do nothing.
- Only **selected** modules are scanned — skipped modules are not part of the effective plan.
- The section is omitted entirely when a profile has no orphans (existing clean reports unchanged).

Implementation: `drift.CheckOrphans(plan, layers []ModuleLayer)` walks each provided layer dir (a pure local-FS pass, no live-system probes), building the referenced set from the plan. `cmd/status.go` derives the layers — `modules/<dir>`, `hosts/<hostname>/modules/<dir>`, `users/<username>/modules/<dir>` — for every selected module and appends the findings before rendering. The `orphans` section renders last in the fixed section order, and orphan findings count as drift in the summary line.

## Acceptance Criteria

- [x] Unreferenced files in any layer of a selected module appear in `orphans:`, per module and layer.
- [x] `module.toml`, entry sources, and symlink-each direct children are not orphans; nested files under a symlink-each source are.
- [x] No orphans → section omitted; pre-existing status output unchanged.
- [x] Orphans render after the built-in sections and count in the drift summary.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Orphan cleanup (deletion/moving) — status reports; the user decides (or `dotdrift onboard` adopts a file).
- Orphans in unselected/disabled modules.
- A `--orphans` filter flag.

## Notes

- Tests: `internal/drift/orphans_test.go` (unreferenced per layer, symlink-each children referenced vs nested, per-module attribution, clean module omits section, render order), `cmd/status_test.go` `TestStatus_reportsOrphans` (end-to-end through the command).
