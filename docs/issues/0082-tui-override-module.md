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
- **Closing commits**: b5ec0bd, b8af99c, 26957f7, 24bbd82

## Summary

A user could not create a host/user overlay for an existing module from
the TUI: the `m` menu scaffolded, moved, and deleted modules, but
starting an overlay meant hand-creating `users/<you>/modules/<dir>/` in
a file manager. The `m` menu gains an override entry; manage writes
refresh the nav; overlay tabs state the merge rule.

## Details

Four tasks, TDD-first:

- **Service op `OverrideModule(app, fromLayer, toLayer)`** mirrors the
  0072 module ops: refused when the source layer has no such module or
  the target layer already has one; creates the target module dir and
  writes a comment-only `module.toml` naming the base module and the
  merge rule. The seed is comments, not a copy: an empty overlay
  overrides nothing, while a copied base would restate every field and
  silently pin them against later base edits (overlays win when set).
  Added to the `ConfigEditor` doorway.
- **Manage dialog entry** (`override module`): listed only when the
  selected module has a higher layer — base can be overridden in the
  host and user overlays, a host overlay in the user overlay, a user
  module has nothing above it and the entry hides. The target is one
  `left/right` choice row reusing the move plumbing; the confirm gate
  says `(the overlay starts empty)`. Success reports
  `created overlay of <app> in <layer>`.
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

Deliberately out of scope: auto-jumping the workspace onto the new
overlay (after the reload the child is one enter away), forking dotfile
sources into the overlay (that is a fork, not an override), root-owned
user overlays (0029's classification).

## Acceptance

- [x] `OverrideModule("m", "", "users/kim")` creates the seed file, the
  source module stays put, the seed names the merged families and
  carries no TOML keys (`TestModuleOps_overrideSeedsEmptyOverlay`).
- [x] Collision and missing-source refusals error and create nothing.
- [x] The menu lists `override module` for a base module with overlays
  available and hides it for a user-layer module
  (`TestManage_menuEntriesForModule`,
  `TestManage_noOverrideEntryForUserModule`).
- [x] The flow creates `users/cri/modules/demo/module.toml` with the
  seed, the gate names the empty start, the report names the overlay
  (`TestManage_overrideSeedsUserOverlay`); a taken target refuses
  (`TestManage_overrideCollisionRefuses`).
- [x] A manage create lands in `nav.modules` after the op settles,
  without a restart (`TestManageWrite_reloadsNav`).
- [x] The hint renders on a user tab and not on the base tab
  (`TestWorkspace_overlayHintOnNonBaseTab`).

## Verification

Fresh runs on the final tree: `go test ./... -count=1` — 20 packages
ok, 0 FAIL; `go vet ./...` exit 0; gofmt clean on touched files;
golangci-lint `run ./internal/...` — 0 issues. Golden sweep: 1 frame
regenerated in task 2 (`manage-menu.golden`, the diff audited — the
modal grows one row and lists `override module`); the task 4 sweep is
a no-op because no golden keeps an overlay tab active. TDD red→green:
every task failed first on the missing API (`OverrideModule undefined`,
`overrideTargets undefined`, the nav assertion, the hint assertion).
The RED runs caught two of my test bugs (a skipped enter that arms the
confirm gate; an `L` press with focus still on nav — the binding is
work-scope) and no production bugs; the green run then caught the hint
truncating at 100 columns, fixed by shortening the line.

## Notes

The seed wording is the contract's short form: merged families are
packages, tools, dotfiles, hooks, mounts, smb (ADR/contract 8 covers
dir self-containment; the merge rule is the loader's representative
layer). If the merge families ever change, the seed line, the workspace
hint, and tui.md change together.
