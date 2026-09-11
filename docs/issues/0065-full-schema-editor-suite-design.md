---
type: Issue
title: Full-schema editor suite design
description: Design the structured editors for every module.toml section, including the unsaved-changes/validation model that keeps strict-schema and layered-merge guarantees.
tags: [wayfinder, grilling, tui, editors]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0065: Full-schema editor suite design

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [profile layout](../product/profile-layout.md), [contract invariants 7, 19](../product/contract.md)
- **Related code**: [`internal/profile/spec.go`](../../internal/profile/spec.go), [`internal/tui/tui.go`](../../internal/tui/tui.go)
- **Blocked by**: [0062 TUI information architecture](0062-tui-information-architecture.md)
- **Closing commits**: none

## Question

The design commits to **first-class structured editors for every
`module.toml` section** — dotfiles (whole-file AND edit entries),
packages, tools, mounts, smb, systemd.units, hooks, secrets,
`bootstrap.*`, plus module-level keys (`scope`, `disable`, `when`) and
the layer/module management operations (create/move/remove modules and
`hosts/`/`users/` layers, manage `bootstrap.users` OS accounts). How is
that suite shaped so it stays one design instead of fourteen?

- The common editor frame: field-level forms over the strict schema
  (contract 19 — unknown keys are load-time errors, so the editor can
  never emit one), section-level save semantics, whole-entry-by-name
  merge made visible when an overlay shadows a base declaration
  (contract 7).
- The unsaved-changes model: where edits live before write (in-memory
  `ModuleConfig` diff? staged TOML?), validation timing (per-field,
  per-save), and what "save" means for layered files — the editor edits
  ONE layer's `module.toml`; how the UI says which.
- Reuse of the generate spec-builders (`internal/tui`'s
  MountsWizard/SmbWizard state machines and `internal/generate`
  assembly) as the write path for mounts/smb editors, extending the
  same pattern to other sections — the absorption of the wizard starts
  here conceptually.
- The hard editors: dotfiles entries (targets, sources, modes,
  edit-entry line/block/template variants), `when` expressions
  (combinators — issues 0007/0008), secrets (env indirection). For each:
  form shape, validation story, what is deliberately deferred.
- Layer/module management: creating `hosts/<h>/modules/<m>/` vs
  `users/<u>/` overlays; moving a module between layers (what that even
  means to file layout); deleting safely (what references break).
- OS accounts (`bootstrap.users`): editing users/groups declarations vs
  the fact that they only take effect at apply.
