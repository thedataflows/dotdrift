---
type: Issue
title: Service-layer architecture &amp; CLI migration
description: Design the package layout, seams, error model, and the big-bang migration of every cmd/ command onto the product service API.
tags: [wayfinder, grilling, api, architecture]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0061: Service-layer architecture &amp; CLI migration

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract](../product/contract.md), [`docs/engineering/package-layout.md`](../engineering/package-layout.md)
- **Related code**: [`cmd/`](../../cmd/), [`internal/profile/`](../../internal/profile/), [`internal/resolve/`](../../internal/resolve/), [`internal/generate/`](../../internal/generate/)
- **Blocked by**: [0060 IDL choice &amp; Go derivation](0060-idl-choice-go-derivation.md)
- **Closing commits**: none

## Question

The service layer is a **big-bang doorway**: the design must make every
command — CLI and TUI — a client of one product API. What does that
layer look like, and how does `cmd/` migrate onto it?

- Package placement and naming (e.g. `internal/product/`,
  `internal/api/`, `internal/service/`) against
  `docs/engineering/package-layout.md` conventions and the existing
  seams (`internal/tui`'s spec-builders already model the pattern).
- Service decomposition: read models (plan, status, modules, detect,
  diff), write models (onboard, restore, generate, profile-file
  editing), and sessions (apply with streaming events, cancel). Which
  existing internals do they wrap vs absorb?
- How `cmd/` thins: kong stays the CLI parser; commands become thin
  adapters over service calls. What happens to flags like
  `plan --json` (service returns typed data; rendering stays in `cmd/`)?
- Error model: how resolve-time errors, strict-schema load errors, and
  apply step failures surface as typed API errors both the CLI printer
  and the TUI can render.
- The TTY boundary: apply sessions carry the terminal-handoff hooks the
  TUI needs (ticket 0064's output) without the service knowing about
  bubbletea.
- Migration slicing: the order `cmd/` commands move onto the layer such
  that the repo is always green (this is *design* of the migration, not
  doing it).
