---
type: Issue
title: Surface TTY handover for real steps
description: Make mise-runner steps emit the child commands that need the terminal (sudo elevation, interactive hooks) through the session's Handover seam, with hook sub-steps.
tags: [implementation, apply, tui, mise]
timestamp: 2026-09-12T00:00:00Z
---

# ISSUE 0071: Surface TTY handover for real steps

- **Type**: task
- **Status**: open
- **Priority**: high
- **Labels**: [implementation]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [0069 apply-session service core](0069-implement-apply-session-service-core.md), [0064 D3/D4/D9](0064-apply-session-tty-suspend-design.md), [contract invariants 12, 13](../product/contract.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), `internal/service/`
- **Blocked by**: [0069 Implement apply-session service core](0069-implement-apply-session-service-core.md)
- **Closing commits**: none

## Question

0069 ships the `Handover` seam and tests it with fake steps; no real
step declares a terminal need yet. This ticket makes the honest
classification real: the mise-runner steps surface the child commands
that truly need the terminal — sudo-elevated dotfiles/system applies
(`DotfilesApplySudo` path), interactive `mise run` hook tasks — through
the session's handover wrapper (session-enforced `Setpgid` + nil stdio,
consumer-run), so `Preview()` reports true `NeedsTTY` + reasons and a
TUI can `tea.ExecProcess` them. Includes D3's hook sub-steps: split
`HooksStep` so each hook command's start/fail is observable
(`StepStarted.Sub{Index, Total, Command}`), and D4's classification
grounded in the actual per-step elevation/interactivity decisions
(euid, writability probes, `interactive = true` declarations) instead
of the placeholder interface.

## Acceptance Criteria

- [ ] Steps that elevate or declare interactive hooks implement the
      handover interface; their children run through `Handover` with
      session-enforced process-group semantics (contract 12/13 hold).
- [ ] `Preview()` reasons name the real cause (sudo path, interactive
      hook) per step.
- [ ] Hook steps emit per-command sub-step events.
- [ ] CLI passthrough mode stays byte-identical (handover wiring equals
      today's direct-fd path).
