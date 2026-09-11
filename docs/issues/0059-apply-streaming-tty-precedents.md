---
type: Issue
title: Apply streaming &amp; TTY precedents
description: Research how bubbletea apps stream long-running subprocesses and hand back the terminal for sudo/interactive steps, to shape the apply-session model of the IDL.
tags: [wayfinder, research, tui, apply, tty]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0059: Apply streaming &amp; TTY precedents

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:research]
- **Assignee**: wayfinder research subagent
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariants 12 and 13](../product/contract.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`cmd/apply.go`](../../cmd/apply.go)
- **Blocked by**: none (frontier)
- **Closing commits**: pending

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

## Resolution

Researched against bubbletea v1.3.6 (the version dotdrift pins), creack/pty,
lazygit, and gh-dash sources, grounded in `cmd/apply.go` / `internal/mise` /
`internal/apply` / `internal/state`. Findings:
[0059 research doc](../research/0059-apply-streaming-tty-precedents.md).

- `tea.ExecProcess` (v1.3.6) is the handover tool for sudo/interactive
  steps: it releases the terminal, runs the child on the real one
  (so sudo prompts and `interactive = true` hook tasks work), and
  restores + repaints on exit, delivering the error through the callback.
  `ReleaseTerminal`/`RestoreTerminal` are the manual pair for non-exec
  blocking work. `tea.ExecFramebuffer` does **not** exist in any
  bubbletea version — `tea.Exec`'s `ExecCommand` interface is the real
  extension point.
- Streaming shape: reader goroutine → bounded line ring → `tea.Tick`
  coalescing → bubbles viewport tail (per-chunk messages starve the
  unbuffered event loop); child color survives only via a PTY
  (mise/paru emit no ANSI on pipes — verified in this repo), and a
  spawned PTY is not the controlling terminal, so it can't carry sudo
  prompts — real-terminal handover only.
- Precedents: lazygit = suspend → run on real terminal with `+ argv`
  echo → resume for interactive commands, per-view buffer managers +
  PTYs for streamed panes, process-tree reaping on exit; gh-dash
  (bubbletea) = per-task Start/Finished/Error registry with spinners,
  subprocess output discarded so it can't corrupt the display.
- dotdrift reality: the 9 named pipeline steps + per-hook index + exit
  codes are the progress keys — the repo already never parses mise
  output for decisions; which steps need a TTY (sudo elevation,
  interactive hooks) is pre-computable from euid, writability probes,
  and plan declarations; pane runs must use `--yes` (a null-stdin mise
  prompt answers "No" and still exits 0, recording a drifted step).
- Cancel: `Pipeline` already preserves the cursor on abort (contract 2),
  so the TUI only presents "stopped at `<step>` — rerun to resume"; gap
  found: `runOp`'s streaming path lacks the `Setpgid` process-group kill
  `runContextEnv` has, and `ApplyCmd.Run` uses `context.Background()` —
  the apply-session must funnel one cancellable ctx with process-group
  kills through the service layer (`state.TryLock` covers the
  "already running" case).
