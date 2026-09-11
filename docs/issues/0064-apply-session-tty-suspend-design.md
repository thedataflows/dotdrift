---
type: Issue
title: Apply session &amp; TTY suspend design
description: Design the apply-session shape (events, progress, cancel, terminal handoff) the IDL carries so apply streams inside the TUI.
tags: [wayfinder, grilling, apply, api]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0064: Apply session &amp; TTY suspend design

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariants 2, 12, 13](../product/contract.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), [`internal/mise/`](../../internal/mise/), [`internal/state/`](../../internal/state/)
- **Blocked by**: [0059 Apply streaming &amp; TTY precedents](0059-apply-streaming-tty-precedents.md)
- **Closing commits**: none

## Question

What is the apply-session model the service layer exposes, such that a
UI can stream an apply, hand the terminal back when a step demands a
real TTY, and never lie about pipeline state?

- Session lifecycle: start (selection, section flags, `--force`,
  backup options) → event stream (step started/finished, per-step
  output chunks, privilege prompts) → terminal outcome; who owns the
  resume cursor (contract 2) when a session dies mid-run.
- Event vocabulary: how much of mise's raw output passes through vs is
  keyed into structured step events (ground this in what the research
  found mise actually emits and in `internal/mise`'s invocation seams).
- TTY handoff as a *contract* concept: the API must expose "this step
  needs the terminal" as something a UI reacts to (bubbletea
  suspend/resume, or `tea.ExecProcess`-style callback seams the service
  invokes) — without the service importing any UI package. What is the
  exact seam (callback interface in the service, exec func injected by
  the caller)?
- Cancel: what cancel means mid-step (kill process group? finish step
  then stop?), and what the TUI shows about the half-applied state.
- Concurrency: one session at a time (the sidecar lock — contract 11 —
  makes concurrent applies impossible; the API must say so and the TUI
  must show it).
