---
type: Issue
title: Override module from the TUI — seed an overlay for an existing module in a higher layer
description: The manage menu gains an override entry that creates users/<u>/modules/<dir> (or hosts/<h>) seeded with a comment-only module.toml; the nav re-reads after any manage write so new dirs appear without a restart; overlay tabs state the merge rule in one dim line.
tags: [issue, tui, service, ergonomics]
timestamp: 2026-09-21T15:11:38Z
---

# ISSUE 0082: Override module from the TUI

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, service, ergonomics]
- **Assignee**: none
- **Related**: [0072](0072-module-management-dialogs.md), [0065](0065-full-schema-editor-suite-design.md)
- **Related code**: [`internal/service/config.go`](../../internal/service/config.go), [`internal/tui/manage.go`](../../internal/tui/manage.go), [`internal/tui/modals.go`](../../internal/tui/modals.go), [`internal/tui/compositor.go`](../../internal/tui/compositor.go), [`internal/tui/workspace.go`](../../internal/tui/workspace.go)
- **Closing commits**: b5ec0bd, b8af99c, 26957f7, 24bbd82, f65d3b3

## Summary

A user could not create a host/user overlay for an existing module from
the TUI: the `m` menu scaffolded, moved, and deleted modules, but
starting an overlay meant hand-creating `users/<you>/modules/<dir>/` in
a file manager. The `m` menu gains an override entry; manage writes
refresh the nav; overlay tabs state the merge rule.

## Details

Four tasks, TDD-first, then one simplification pass that replaced the
second task's UI (f65d3b3):

- **Service op `OverrideModule(app, fromLayer, toLayer)`** mirrors the
  0072 module ops: refused when the source layer has no such module or
  the target layer already has one; creates the target module dir and
  writes a comment-only `module.toml` naming the base module and the
  merge rule. The seed is comments, not a copy: an empty overlay
  overrides nothing, while a copied base would restate every field and
  silently pin them against later base edits (overlays win when set).
  Added to the `ConfigEditor` doorway.
- **Manage dialog entry** (`override module`) — **superseded by the
  `O` key**: the first cut put override behind the `m` menu (entry, one
  `left/right` layer choice row, confirm gate). That was five-plus
  keystrokes of ceremony for a non-destructive one-file creation, so the
  dialog path was deleted and `O` on the nav pane does it in one step:
  the selected module gains a `users/<you>` overlay at once (the layer
  that wins the merge order; a host overlay stays available through the
  service seam but nothing in the TUI asked for it), the footer reports
  `created overlay of <app> in users/<you>`, and the nav reload lands
  the workspace on the new file — cursor and tab on it, ready to edit.
  Refusals surface in the footer: `select a module first`, `<app>
  already lives in your user layer`, the op's own taken-target error.
  There is no confirm gate: the seed is comments only, overrides
  nothing, and delete module undoes it.
- **Nav reload after manage writes**: `dialogModal` gained a `reload`
  cmd hook; the manage dialogs set it to the compositor's `reloadNav`
  (the startup profile read, factored out of `Init`). A successful
  manage write (create, move, delete, override) returns the reload, so
  the new directory appears in the tree without a restart — this fixes
  the same staleness in the three pre-existing ops. The dialog stays
  open showing its report while the tree refreshes behind it.
- **Overlay hint line**: a workspace tab whose layer is not base shows
  one dim line under the title — `overlay: only packages, tools,
  dotfiles, hooks, mounts, smb merge; the rest comes from base` —
  because the loader merges only those six families while meta, scope,
  description, and when always come from the base file (the
  representative-layer rule); editing those in an overlay was a silent
  no-op and now says so. Kept under 100 columns so it survives narrow
  panes.

Landing the workspace on the new overlay — rejected as scope in the
first pass, delivered by the simplification — is the `pendTab` handoff:
the override stashes the new layer dir, and the nav reload that reveals
it consumes the stash, refreshes the module's tabs from the fresh read,
and pulls the nav cursor onto the child row. Still out of scope: forking
dotfile sources into the overlay (that is a fork, not an override),
root-owned user overlays (0029's classification).

## Acceptance

- [x] `OverrideModule("m", "", "users/kim")` creates the seed file, the
  source module stays put, the seed names the merged families and
  carries no TOML keys (`TestModuleOps_overrideSeedsEmptyOverlay`).
- [x] Collision and missing-source refusals error and create nothing.
- [x] `O` on a base module creates `users/cri/modules/demo/module.toml`
  with the seed, reports it in the footer, and after the reload the
  workspace sits on the new overlay with the nav cursor on its child row
  (`TestOverride_oneKeyCreatesAndLands`).
- [x] Refusals: no selection, a module already in the user layer, a
  taken target (`TestOverride_refusals`).
- [x] A manage create lands in `nav.modules` after the op settles,
  without a restart (`TestManageWrite_reloadsNav`).
- [x] The hint renders on a user tab and not on the base tab
  (`TestWorkspace_overlayHintOnNonBaseTab`).

## Verification

Fresh runs on the final tree: `go test ./... -count=1` — 20 packages
ok, 0 FAIL; `go vet ./...` exit 0; gofmt clean on touched files;
golangci-lint `run ./internal/...` — 0 issues. Golden sweep: 1 frame
regenerated in task 2 (`manage-menu.golden`, the diff audited — the
modal grows one row and lists `override module`), then 1 more in the
simplification pass (the same frame shrinks back, the audited diff is
exactly the removed `override module` row); the hint task's sweep is a
no-op because no golden keeps an overlay tab active. TDD red→green: the
original tasks failed first on the missing API (`OverrideModule
undefined`, `overrideTargets undefined`, the nav assertion, the hint
assertion); the simplification's tests failed first on the unbound `O`
key. The RED runs caught three of my test bugs (a skipped enter that
arms the confirm gate; an `L` press with focus still on nav; a
user-layer-only fixture that the loader rightly never lists, so the
"already yours" refusal must be reached from the user child row) and no
production bugs; the green runs caught the hint truncating at 100
columns in the first pass and my over-strict golden-width assertion in
the second.

## Notes

The seed wording is the contract's short form: merged families are
packages, tools, dotfiles, hooks, mounts, smb (ADR/contract 8 covers
dir self-containment; the merge rule is the loader's representative
layer). If the merge families ever change, the seed line, the workspace
hint, and tui.md change together.
