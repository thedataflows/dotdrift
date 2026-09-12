---
type: Issue
title: Assemble the TUI design set
description: Destination-closing ticket — write tui.md, the API contract doc, the ADRs, the M14 proposal, and the glossary additions from the resolved tickets.
tags: [wayfinder, task, tui, design, docs]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0067: Assemble the TUI design set

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [wayfinder:task]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md)
- **Related code**: none yet
- **Blocked by**: [0061 Service-layer architecture](0061-service-layer-architecture-cli-migration.md), [0063 Two-pane shell prototype](0063-two-pane-shell-prototype.md), [0064 Apply session &amp; TTY suspend design](0064-apply-session-tty-suspend-design.md), [0065 Full-schema editor suite design](0065-full-schema-editor-suite-design.md), [0066 Wizard absorption &amp; contract #15 amendment](0066-wizard-absorption-contract-15.md)
- **Closing commits**: 9332a82 (service API contract + product spec), 9d4f117 (ADR-0008), 749f042 (M14 proposal + task docs), 6e1ba51 (glossary, README, log)

## Question

Everything upstream is decided — the design set is assembled and
reviewable. What ships, where, and is it approved?

- `docs/product/tui.md` (OKF): the product spec — command surface, IA,
  screens, editors, apply UX — written *from the resolutions*, not
  re-litigating them.
- The API contract doc from ticket 0060/0061's decisions (location
  fixed there), normative prose over the Go service layer — the whole
  product surface.
- ADRs per ticket 0066's decision (service-layer doorway; wizard
  absorption).
- M14-TUI milestone proposal in `docs/milestones/` (+ `docs/tasks/` per
  conventions), sliced from the design so implementation can start as
  ordinary planned work.
- `CONTEXT.md` glossary additions for any terms the design crystallized
  (e.g. service layer, apply session, shell) — glossary only, no
  implementation detail.
- `docs/issues/index.md`, `docs/log.md`, README/cli-surface touch-ups
  that reference the design.
- Approval: the human reviews the set; the map's destination is met when
  it is approved as-is or after agreed amendments. This ticket is the
  last one — when it closes, the way is clear and the map is done.

## Resolution

**Approved as-is, 2026-09-12** — the human reviewed the assembled set
and confirmed without amendments, including the four judgment calls the
assembly made without a grill round: the contract doc lives at
`docs/product/service-api.md` (the product index was the only home with
a convention for normative surfaces after 0060 killed the IDL tree),
ADR-0008 exists (the doorway is ADR-grade; 0066's "one ADR" ruled only
its own scope), M14 slices into the five 0061-D7-ordered tasks with the
config area riding the editors slice, and the task docs name concrete
`Test*` vectors as proposals to sharpen when each task starts.

The set, as committed: [service-api.md](../product/service-api.md) —
the normative Go service-layer contract (areas, the apply session as
implemented, error taxonomy, rendering ownership, rules for change) —
[tui.md](../product/tui.md) (the product spec),
[ADR-0008](../adr/0008-service-layer-doorway.md) plus
[ADR-0007](../adr/0007-generate-cli-only.md),
[m14-tui.md](../milestones/m14-tui.md) with its five TDD task docs,
four glossary terms in `CONTEXT.md`, and the index/README touch-ups.

This was the map's last ticket: with it done,
[0057](0057-dotdrift-tui-design-map.md)'s destination — an approved
design set — is met and the wayfinder map closes. Design is over;
implementation is ordinary milestone work now, starting at M14 task 1
(reads onto the service layer).
