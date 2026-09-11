---
type: Issue
title: Dotdrift TUI design map (wayfinder)
description: Wayfinder map — chart the way to an approved design set for the dotdrift tui command, its service-layer API contract, and the M14 proposal.
tags: [wayfinder, map, tui, api, design]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0057: Dotdrift TUI design map (wayfinder)

- **Type**: task
- **Status**: in-progress
- **Priority**: high
- **Labels**: [wayfinder:map]
- **Assignee**: none
- **Related**: [contract invariant 15](../product/contract.md), [ADR-0003](../adr/0003-tui-shared-palette.md), [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [M13 Generate](../milestones/m13-generate.md)
- **Related code**: [`internal/tui/`](../../internal/tui/), [`cmd/`](../../cmd/), [`internal/profile/`](../../internal/profile/), [`internal/resolve/`](../../internal/resolve/)
- **Closing commits**: none

## Destination

An approved design set for `dotdrift tui`: the product spec
(`docs/product/tui.md`), the normative service-layer API contract doc for
the full product service layer (the front door every command migrates
onto), supporting ADRs, and a proposed M14-TUI milestone breakdown.
Design only — no implementation ships from this map; implementation
happens later as ordinary milestone work.

## Notes

Domain: dotdrift is a Go CLI (kong) that translates layered `module.toml`
profiles into mise configs; mise owns convergence (ADR-0004). The
existing TUI seed is the `generate mounts|smb` wizard
(`internal/tui`): huh forms over pure spec-builder state machines, with
CLI/wizard byte-equivalence pinned by contract invariant 15 and a shared
palette pinned by ADR-0003. bubbletea/bubbles are currently indirect
dependencies (via huh).

Standing preferences settled while charting (grilling rounds 1–2):

- The service layer is a **big-bang doorway**: every command (CLI and
  TUI) migrates onto it; the design includes the migration of all
  `cmd/`.
- The contract is the **Go service layer itself** (interfaces + structs)
  — the formal-IDL preference from charting was revoked during the 0060
  grill: no IDL is checked in; non-Go consumers stay out of scope until
  one exists.
- **Full-schema editors**: first-class structured editors for every
  `module.toml` section.
- Apply runs **inside the TUI, streamed**, with bubbletea
  suspend/resume when a step truly needs the terminal (sudo prompts,
  interactive hooks — contract invariants 12/13).
- The tree manages **profile layers AND OS accounts**
  (`hosts/`, `users/`, `bootstrap.users`).
- The TUI **absorbs** the `generate --tui` wizard (contract 15 gets
  amended).
- The design set ends with the **M14 proposal**.

Skills every session should consult: `grilling` + `domain-modeling` for
grilling tickets, `research` for research tickets, `prototype` for
prototype tickets.

**Wayfinding operations in this tracker** (local markdown,
`docs/issues/`): the map is this issue (`Labels: wayfinder:map`); tickets
are issues 0058–0067 labeled `wayfinder:<type>` with `Related` pointing
here. The tracker has no native blocking: each ticket's `Blocked by`
header field names its prerequisite issues; a ticket is **unblocked**
when every blocker's Status is `done` or `wont-do`; the **frontier** is
open, unblocked, unassigned tickets. **Claim** a ticket by setting
Assignee and Status `in-progress` before any work. **Resolution**: append
a `## Resolution` section carrying the decision, flip Status to `done`
(same edit fills Closing commits), then add one line under Decisions so
far below. Research findings land in `docs/research/` and are linked from
the ticket. Wayfinder tickets deliberately skip the issue template's
Acceptance Criteria: the Question is the body; the Resolution closes the
ticket.

## Decisions so far

- [Apply session & TTY suspend design](0064-apply-session-tty-suspend-design.md):
  `service` apply session as a run handle — `Start` (resolve → `TryLock`
  → classify → `PlanResolved`) on one goroutine streaming a closed
  event vocabulary (hooks as sub-steps; handover is the synchronous
  `Handover(*exec.Cmd)` seam, not an event); `Output io.Writer` picks
  the consumption policy — attached = fd passthrough (byte-identical
  CLI), absent = line-buffered colorless chunk events; `Preview()`
  exposes per-step TTY needs up front and the `interactive = true`
  opt-in keys on handover-available, not raw stdin; cancel = immediate
  process-group kill with the cursor naming the last completed step;
  exactly two new typed errors (`AlreadyRunningError`,
  `SessionCancelledError`); `cmd/apply.go`'s orchestration is absorbed
  wholesale — the apply view-model and output coalescing stay TUI-side
  (0065/0067).
- [Two-pane shell prototype](0063-two-pane-shell-prototype.md): the
  human ran the prototype branch and approved the shell as-is — 30%
  tree with column clamp, border-color focus, origins-as-children, the
  one-line header + status-bar help. The 0062 IA decisions are now
  reacted-upon; the prototype stays on its branch (static colors and
  stubbed editors/apply are recorded simplifications, not patterns to
  copy).
- [TUI information architecture](0062-tui-information-architecture.md):
  charm v2; module-centric tree (Modules/Accounts/Profile, one node per
  module across layers, overlay origins as markers); resolved-by-default
  selection with raw-on-demand; view stack with one active view, dirty
  editor state survives navigation; vim-ish keys, one focused pane,
  `help.Model` help; keys + context menus only (no palette); minimal
  chrome with ADR-0003 registry discipline; accounts as nodes, no
  per-account drift views (ADR-0006 notice only); full-status parity via
  0061's canonical renderer; Profile = plan/status views + onboard/
  restore/generate dialog actions; the plan view's gate is the TUI's
  only write path into convergence.
- [Service-layer architecture & CLI migration](0061-service-layer-architecture-cli-migration.md): `internal/service` — one package, per-area structs (reads/writes/session) composed into a root; wrap the 17 domain packages, absorb `cmd/` orchestration; typed errors at the boundary (`SchemaError`, `StepError`, `AlreadyRunningError`); apply pipeline is service-driven with a `Handover(*exec.Cmd)` TTY seam (0064 designs on it); canonical text renderers shared only where CLI/TUI must be identical; migration slices reads → writes → apply session, always green with byte-identical output; single-flight lock owned by the session.
- [IDL choice & Go derivation](0060-idl-choice-go-derivation.md): no IDL — the Go service layer (interfaces + structs) is the normative contract; operations are interface methods, apply events are Go types (research 0059's vocabulary); surface grows TUI-critical-core-first; versioning becomes ordinary Go package versioning; non-Go consumers deferred until one exists.
- [Apply streaming & TTY precedents](0059-apply-streaming-tty-precedents.md): stream steps into a pane keyed off the pipeline's own step/exit events; hand the real terminal over via `tea.ExecProcess` only for pre-classified TTY steps (sudo prompts, interactive hooks), run pane steps with `--yes`, and cancel through one ctx with process-group kills (cursor already survives aborts).
- [Bubbles & layout inventory](0058-bubbles-layout-inventory.md): charm v2 is GA under `charm.land/*` (v1 frozen), making v1-vs-v2 the first design call; the tree gap closed on v2 only (official tree bubble, Aug 2026); two-pane layout needs nothing beyond lipgloss Join* + WindowSizeMsg; huh embeds as a child model; teatest is still untagged, so pure state machines + golden View() tests stay the testing mandate.

## Not yet specified

- Theming: the *mechanism* by which ADR-0003's palette registry extends
  to bubbles components (registry → lipgloss styles for arbitrary
  components) — beyond the per-surface chrome choices
  [TUI information architecture](0062-tui-information-architecture.md)
  already decides; the inventory (research 0058) landed the facts — v2
  renames the styling APIs — but no ticket owns the mechanism yet.
- Another-host resolution preview: the TUI's module views resolve
  against the current host/user only (0062-D3, matching the CLI's
  read paths); previewing a *different* host's resolved stack would be
  new reads-service surface, deferred until a user story exists.
- Command palette / `:` command line inside the TUI: deferred by
  0062-D6 until [0065's editor suite](0065-full-schema-editor-suite-design.md)
  exists to command; keys + context menus carry M14.
- Per-editor UX specifics (dotfiles entries editor shape, inline
  validation timing, unsaved-changes affordances).
- Large-profile performance (render cost of big trees).

## Out of scope

- The full TUI build — M14+ work this map only *designs* (destination:
  the design set). **Extended 2026-09-12 by human decision**: the
  implementation phase for the designed apply-session service now runs
  under this map as ordinary implementation tickets ([0069](0069-implement-apply-session-service-core.md),
  [0070](0070-migrate-cmd-apply-onto-apply-session.md),
  [0071](0071-surface-tty-handover-for-real-steps.md)); the TUI itself
  still waits for M14 and the 0065–0067 design tail.
- Building non-Go UIs; the contract exists so others *can*, not so this
  effort does.
- Remote/multi-host management and web/GUI frontends.
- Profile schema changes; editors cover the existing `module.toml`
  schema only.
