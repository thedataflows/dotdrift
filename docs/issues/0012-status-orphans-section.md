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
- not the **resolved source** of a `mode = "edit"` entry (its contents are inlined into the block at resolve time, but the file remains the authored source — the entry carries it as `EditSource` for exactly this attribution),
- not inside the **source subtree of a `symlink-each` entry**, attributed to the layer whose `module.toml` declares the entry (mise links directory children wholesale, so the whole nested tree deploys with the entry), and
- not `module.toml` (the manifest itself).

Everything else in the walk is reported: `orphans:` section, item = path relative to the layer module directory, detail `not referenced by [dotfiles]`, module attribution naming the layer — `<dir> [base]`, `<dir> [host:<hostname>]`, `<dir> [user:<username>]` — so a specific host's or user's overlay leftovers are distinguishable from base ones at a glance.

Deliberate semantics:

- The referenced set comes from the **resolved plan** (what apply actually uses), with symlink-each subtrees attributed to the **declaring layer**: resolve picks the highest-precedence layer holding the source path, so an overlay dir at the same rel-path wins resolution (its tree is what apply deploys) — but the declaring layer's tree is the authored reference, and unreferenced files in an overlay that declares nothing stay orphans.
- Nested files under a `symlink-each` source directory (depth ≥ 2) are orphans: `symlink-each` deploys direct children only (documented in [profile layout]), so deeper files do nothing.
- Only **selected** modules are scanned — skipped modules are not part of the effective plan.
- The section is omitted entirely when a profile has no orphans (existing clean reports unchanged).

Implementation: `drift.CheckOrphans(plan, layers []ModuleLayer)` walks each provided layer dir (a pure local-FS pass, no live-system probes), building the referenced set from the plan. `cmd/status.go` derives the layers — `modules/<dir>`, `hosts/<hostname>/modules/<dir>`, `users/<username>/modules/<dir>` — for every selected module and appends the findings before rendering. The `orphans` section renders last in the fixed section order, and orphan findings count as drift in the summary line.

## Acceptance Criteria

- [x] Unreferenced files in any layer of a selected module appear in `orphans:`, per module and layer.
- [x] `module.toml`, entry sources (whole-file, template edit, and `mode = "edit"` resolved sources), and every file under a symlink-each source subtree (declaring layer) are not orphans.
- [x] No orphans → section omitted; pre-existing status output unchanged.
- [x] Orphans render after the built-in sections and count in the drift summary.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Orphan cleanup (deletion/moving) — status reports; the user decides (or `dotdrift onboard` adopts a file).
- Orphans in unselected/disabled modules.
- A `--orphans` filter flag.

## Notes

- **Follow-up fix (same day, dogfooding)**: the first cut flagged `mode = "edit"` source files as orphans — resolve inlines their contents into the block and clears `Source`, so the scan lost the reference. `resolve.DotfileEntry` gained `EditSource` (the resolved path of the consumed edit source, set during the mode translation in `mergeDotfiles`) and `referencedSources` counts it. Reproduced through the real stack (`profile.Load` → `resolve.Resolve` → `CheckOrphans`, `TestCheckOrphans_realStack`) — symlink-each children were already correct end-to-end; only edit sources leaked.
- **Follow-up fix (same day)**: host/user attribution now names the host/user — `ModuleLayer.Owner` — rendering `[host:cri-pc]` / `[user:cri]` instead of a bare `[host]`/`[user]`, so multi-host/multi-user profiles distinguish whose overlay leaked. Orphan lines also render in a distinct magenta shade on TTY, apart from the live-system drift hues.
- **Follow-up fix (same day, field report)**: symlink-each subtrees were scanned one level deep (direct children only), flagging every nested file (`db/*`, `input/*` under an easyeffects source) — and the resolved-source anchoring was inverted: resolve picks the *host* overlay's dir when one exists at the same rel-path, so referencing the resolved subtree would have flagged the base tree and blessed the overlay. Fix: the referenced subtree is anchored to the **declaring layer** (the layer whose module.toml declares the `[dotfiles]` key, carried as `DotfileEntry.Layer`) — its whole tree at the entry's rel-path counts as referenced at any depth, and files in an overlay that declares nothing stay orphans. Pinned by `TestCheckOrphans_realStackSymlinkEachSubtree` (the reported easyeffects shape end-to-end).
- **Follow-up (same day)**: orphans now render grouped under layer-root headings — `base:`, `hosts/<hostname>:`, `users/<username>:` — one `<module>: <file> - not referenced by [dotfiles]` line each (`Finding.Group` carries the heading; `Module` is the bare module dir). All display em-dashes across CLI output (status resume lines, finding/detail separators, module descriptions) were replaced with plain ASCII hyphens.
- Tests: `internal/drift/orphans_test.go` (unreferenced per layer, symlink-each children referenced vs nested, per-module attribution, clean module omits section, render order, real-stack regression), `cmd/status_test.go` `TestStatus_reportsOrphans` (end-to-end through the command).
