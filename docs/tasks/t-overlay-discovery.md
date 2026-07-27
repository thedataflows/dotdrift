---
type: Task
title: T-overlay-discovery
description: ADR-0001 — discover and resolve overlay-only modules so presence = managed holds per layer.
tags: [task, tdd, profile, resolve, adr]
timestamp: 2026-07-27T00:00:00Z
milestone: m13
---

# Goal

Implement [ADR-0001](/adr/0001-discover-overlay-only-modules.md) for
[generate](/milestones/m13-generate.md): a module existing only under
`hosts/<hostname>/modules/<dir>/` or `users/<username>/modules/<dir>/`
is discovered and selected like a base module, so host-scoped generated
content (e.g. machine-specific mounts identified by disk UUID) needs no
meaningless base stub. Two slices: T1 profile discovery, T2 resolve
merge.

# Tests first

## T1 — profile discovery

- `TestDiscover_hostOnlyModuleSelected` /
  `TestDiscover_userOnlyModuleSelected` — overlay-only modules are
  selected by presence (new fixture `testdata/profiles/overlayonly`).
- `TestDiscover_overlayOfBaseNotDuplicated` /
  `TestDiscover_basePreferredRepresentative` — layers dedup by directory
  name; the representative path/config is the base layer's.
- `TestDiscover_conflictingIDsAcrossLayersStillFatal` — two different
  directory names resolving to the same declared `id` stay fatal.
- `TestDiscover_missingOverlayDirsTolerated` — absent host/user
  `modules/` directories (and empty hostname/username facts, which skip
  that layer's scan) are not errors.
- Regression locks on the existing discovery tests
  (`TestDiscoverModules_*` unchanged behavior for base-only profiles).

## T2 — resolve merge

- `TestResolve_overlayOnlyModuleMergesEmptyBase` — absent layers are
  treated as empty during merge; packages/tools/dotfiles/hooks resolve
  from whichever layers exist (RED-proven: host-layer winner label).
- `TestResolve_overlayOnlyModuleScopeFromDiscoveryLayer` — module-level
  metadata comes from the representative (discovery) layer.
- `TestResolve_baseScopePreferredOverOverlay` — overlay `scope`
  declarations are ignored (regression lock).
- `TestResolve_moduleIDIsDirNameForOverlayOnly` — layer directories key
  on the module directory name, never the declared `id` (host → user
  overlay keying).
- `TestResolve_malformedOverlayModuleToml_stillErrors` — malformed
  overlay `module.toml` errors still propagate naming module id and
  overlay path.

# Implementation notes

- `discover()` scans `hosts/<hostname>/modules/` and
  `users/<username>/modules/` in addition to `modules/`; empty
  hostname/username facts skip that layer's scan. `discover`/
  `loadModule` live in `internal/profile/discover.go` (split out of
  `profile.go`, which was over the 250 pure-LOC ceiling pre-change).
- Resolve derives layer paths from `p.Root` + the module directory name
  (`rootFromModule` deleted) so overlay-only modules merge against an
  absent/empty base layer.
- The representative module config (first layer in base → host → user
  order containing the directory) supplies `scope`, `id`, `app`, and
  `when`; overlays merge only the per-entry sections.
- This repairs a latent dead path: `onboard --host` already wrote
  host-layer-only modules that resolve never selected.
- Behavioral consequence (contract invariant 1): any pre-existing
  `hosts/*/modules/<id>` directory in the wild becomes live
  configuration where it was previously inert.

# Docs

- ADR-0001; contract invariant 1 (per-layer presence); profile-layout
  selection/validation rules; merge-rules overlay-only section + the
  representative-layer `scope` row.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
