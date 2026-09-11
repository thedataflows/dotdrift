---
type: Issue
title: Apply streaming &amp; TTY precedents
description: Research how bubbletea apps stream long-running subprocesses and hand back the terminal for sudo/interactive steps, to shape the apply-session model of the IDL.
tags: [wayfinder, research, tui, apply, tty]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0059: Apply streaming &amp; TTY precedents

- **Type**: task
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [wayfinder:research]
- **Assignee**: wayfinder research subagent
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariants 12 and 13](../product/contract.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`cmd/apply.go`](../../cmd/apply.go)
- **Blocked by**: none (frontier)
- **Closing commits**: none

## Question

The TUI will run `apply` inside the app, streamed, and suspend/resume
the bubbletea program when a step needs a real terminal (contract 12:
interactive hooks connect to the terminal; contract 13: sudo prompts on
mise's privileged paths). What are the proven mechanics and precedents,
so the apply-session design (ticket 0064) shapes the IDL correctly?

- bubbletea's official mechanisms and their exact trade-offs:
  `tea.ExecProcess`, `tea.ExecFramebuffer` (if real), `ReleaseTerminal`/
  `RestoreTerminal` — when each is the right tool for sudo/interactive
  steps.
- Streaming patterns for long-running subprocesses: piping stdout/stderr
  through `tea.Cmd` messages, backpressure, PTY wrappers (creack/pty et
  al.), and how apps render incremental output (viewport tail, log
  pane).
- Precedents in real TUIs (lazygit's command runner, gh-dash and
  similar): how they structure "run long thing, show output, keep UI
  alive, cancel safely".
- What mise itself emits during `bootstrap`/`dotfiles apply`/tasks that
  a stream can key progress off (line shapes, exit codes, `--yes`
  behavior) — grounded in this repo's `internal/mise` invocation code.
- Cancel semantics: what happens to a half-applied pipeline when the
  user aborts mid-run (resume cursor exists — contract 2 — how should
  the TUI present it).

Findings land in `docs/research/0059-apply-streaming-tty-precedents.md`,
linked from this ticket.
