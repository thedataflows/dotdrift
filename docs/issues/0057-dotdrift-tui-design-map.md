---
type: Issue
title: Dotdrift TUI design map (wayfinder)
description: Wayfinder map — chart the way to an approved design set for the dotdrift tui command, its service-layer API and formal IDL, and the M14 proposal.
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
(`docs/product/tui.md`), the normative formal IDL + API contract doc for
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
- A **formal IDL** (checked in) is the normative contract even before
  any transport exists.
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

## Not yet specified

- Theming: how ADR-0003's palette registry extends to bubbles components
  (no decision until the bubbles inventory lands).
- Testing strategy for the new TUI: pure state machines vs `teatest`
  driver tests; what the design mandates.
- API versioning of the IDL and its error model details.
- A status/drift view inside the TUI (read-only surface over plan/status
  read models) — scope not yet pinned.
- Multi-account presentation: how the superuser overlay and other
  accounts (contract 17's notices) appear in the tree.
- Per-editor UX specifics (dotfiles entries editor shape, inline
  validation timing, unsaved-changes affordances).
- Large-profile performance (render cost of big trees).

## Out of scope

- Implementing the TUI, service layer, or IDL tooling — that is M14+
  work this map only *designs* (destination: the design set).
- Building non-Go UIs; the contract exists so others *can*, not so this
  effort does.
- Remote/multi-host management and web/GUI frontends.
- Profile schema changes; editors cover the existing `module.toml`
  schema only.
