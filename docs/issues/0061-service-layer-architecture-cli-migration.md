---
type: Issue
title: Service-layer architecture &amp; CLI migration
description: Design the package layout, seams, error model, and the big-bang migration of every cmd/ command onto the product service API.
tags: [wayfinder, grilling, api, architecture]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0061: Service-layer architecture &amp; CLI migration

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract](../product/contract.md), [`docs/engineering/package-layout.md`](../engineering/package-layout.md)
- **Related code**: [`cmd/`](../../cmd/), [`internal/profile/`](../../internal/profile/), [`internal/resolve/`](../../internal/resolve/), [`internal/generate/`](../../internal/generate/)
- **Blocked by**: none (frontier — 0060 resolved: the contract is the Go service layer itself, no IDL)
- **Closing commits**: pending

## Question

The service layer is a **big-bang doorway**: the design must make every
command — CLI and TUI — a client of one product API. What does that
layer look like, and how does `cmd/` migrate onto it?

- Package placement and naming (e.g. `internal/product/`,
  `internal/api/`, `internal/service/`) against
  `docs/engineering/package-layout.md` conventions and the existing
  seams (`internal/tui`'s spec-builders already model the pattern).
  0060's resolution makes this layout carry API versioning too — there
  is no IDL to do it.
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

## Decisions

Round 1 — accepted 2026-09-11:

- **D1 Placement**: `internal/service/`, no version directory. Go-module
  releases are the only versioning that can apply to in-module consumers;
  breaking-change escape hatch is the standard copy-forward
  (`internal/service/v2/`), to be noted in the layout doc. No `api/` name.
- **D2 Shape**: per-area service structs in one package (reads:
  plan/status/modules/detect/diff; writes:
  onboard/restore/generate/profile-editing; session: apply), composed into
  a root the front ends hold. No shipped interface hierarchy — consumers
  define the narrow interfaces they fake.
- **D3 Errors**: exported typed errors at the service boundary
  (`SchemaError{Path, Line…}`, `StepError{Name, ExitCode, Resumable…}`),
  matched with `errors.Is/As` at both front ends; error types live in
  `internal/service`, domain packages keep internal errors and the service
  translates at its boundary.
- **D4 TTY boundary**: service-driven pipeline + `Handover(*exec.Cmd) error`
  callback (CLI: today's fds-to-terminal wiring; TUI: `tea.ExecProcess`,
  per research 0059). The classification query ("which steps need the
  TTY") is part of the session API. Ticket 0064 designs on this seam.
- **D5 Rendering**: typed data + canonical text renderers in the service
  layer only for surfaces both front ends show identically (plan report,
  diff, status summary); `--json` is a dumb marshal in `cmd/`; all
  TUI-styled rendering stays TUI-side.

Round 2 — accepted 2026-09-11:

- **D6 Wrap vs absorb**: wrap the domain, absorb the orchestration. The
  17 `internal/` packages keep their deep-module boundaries and public
  APIs untouched; what moves into `internal/service` is the coordination
  logic currently living in `cmd/` (plan assembly, report assembly,
  apply step wiring) plus the D3 error translation.
- **D7 Migration slicing**: reads first (modules/status/plan/diff/detect
  — proves the skeleton and error model on real failures), then writes
  (init/onboard/restore/generate/profile-editing), apply session last.
  Per-slice invariant: separate session, `go test ./...` green, CLI
  output byte-identical (D5's canonical renderers make that checkable),
  kong flags unchanged.
- **D8 Single-flight ownership**: the apply session owns the state lock
  — session start takes `state.TryLock` and returns a typed
  `AlreadyRunningError`; reads are lock-free; front ends render the
  typed error and hold zero lock logic.

## Resolution

**The product service layer is `internal/service`: one package, per-area
service structs composed into a root the front ends hold. No IDL, no
shipped interface hierarchy, no version directory.** The Go service layer
is the normative contract (0060); this ticket decides its shape and the
migration path for `cmd/`.

- **Shape** (D1/D2/D6): reads (plan/status/modules/detect/diff), writes
  (onboard/restore/generate/profile-editing), session (apply) as area
  structs; consumers define the narrow interfaces they fake. The 17
  domain packages keep their boundaries — the service layer is the
  missing *coordination* module, wrapping domain and absorbing the
  orchestration currently living in `cmd/`.
- **Errors** (D3): exported typed errors at the service boundary
  (`SchemaError`, `StepError{Name, ExitCode, Resumable}`,
  `AlreadyRunningError`), matched with `errors.Is/As` at both front
  ends; types live in `internal/service`, which translates domain
  errors at its boundary. Today's wrapped-string errors cannot carry
  the structure a TUI must render.
- **TTY seam** (D4): service-driven apply pipeline with a
  `Handover(*exec.Cmd) error` callback — CLI passes today's
  fds-to-terminal wiring, the TUI passes `tea.ExecProcess` (research
  0059). Step-TTY classification is part of the session API.
  [Ticket 0064](0064-apply-session-tty-suspend-design.md) designs the
  session on this seam.
- **Rendering** (D5): canonical text renderers live in the service layer
  only where CLI and TUI must be fact-identical (plan report, diff,
  status summary); `--json` stays a dumb marshal in `cmd/`; TUI-styled
  rendering never shares.
- **Migration** (D7/D8): slices in the order reads → writes → apply
  session, each green with byte-identical CLI output; single-flight is
  the session's job via `state.TryLock`.
- **Versioning** (D1): ordinary Go package versioning; the
  copy-forward escape hatch (`internal/service/v2/`) is noted in
  `docs/engineering/package-layout.md` when the package is created
  (this map ships no implementation).

Consequence carried consciously: non-Go UIs are deferred (map
out-of-scope), not served — if a consumer ever exists, an exchange
format is derived from the Go types at that point (0060's resolution).

Unblocks nothing immediately ([0067](0067-assemble-tui-design-set.md)
still waits on 0063/0064/0065/0066); removes the versioning/error-model
fog item from the map. Remaining frontier: [0062 TUI information
architecture](0062-tui-information-architecture.md), [0064 apply
session](0064-apply-session-tty-suspend-design.md).
