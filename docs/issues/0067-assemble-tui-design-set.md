---
type: Issue
title: Assemble the TUI design set
description: Destination-closing ticket — write tui.md, the IDL + API contract doc, the ADRs, the M14 proposal, and the glossary additions from the resolved tickets.
tags: [wayfinder, task, tui, design, docs]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0067: Assemble the TUI design set

- **Type**: task
- **Status**: open
- **Priority**: high
- **Labels**: [wayfinder:task]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md)
- **Related code**: none yet
- **Blocked by**: [0061 Service-layer architecture](0061-service-layer-architecture-cli-migration.md), [0063 Two-pane shell prototype](0063-two-pane-shell-prototype.md), [0064 Apply session &amp; TTY suspend design](0064-apply-session-tty-suspend-design.md), [0065 Full-schema editor suite design](0065-full-schema-editor-suite-design.md), [0066 Wizard absorption &amp; contract #15 amendment](0066-wizard-absorption-contract-15.md)
- **Closing commits**: none

## Question

Everything upstream is decided — the design set is assembled and
reviewable. What ships, where, and is it approved?

- `docs/product/tui.md` (OKF): the product spec — command surface, IA,
  screens, editors, apply UX — written *from the resolutions*, not
  re-litigating them.
- The IDL artifact + API contract doc from ticket 0060/0061's decision
  (location fixed there), normative, with the service surface for the
  whole product.
- ADRs per ticket 0066's decision (service-layer doorway; wizard
  absorption; IDL choice if it earns one).
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
