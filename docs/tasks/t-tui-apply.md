---
type: Task
title: T-tui-apply
description: Apply inside the TUI — the plan gate on Preview, the streamed progress view-model with output coalescing, tea.ExecProcess handover, cancel.
tags: [task, tdd, tui, apply, session]
timestamp: 2026-09-12T00:00:00Z
milestone: m14
---

# Goal

The last M14 slice: apply mode in the TUI over the shipped apply session
(issues 0069–0071). The plan view becomes the gate (the TUI's only write
path into convergence), the session's events stream into a progress
view-model, TTY steps take the real terminal, cancel kills the process
group, and the cancelled/failed states render with contract-2 cursor
semantics.

# Tests first

- Pure state machines + golden `View()`:
  `TestApplyGate_previewAnnouncesTTY` ("N steps will take the terminal" +
  reasons from `Preview()`), `TestApplyGate_confirmStartsSession` /
  `_declineWritesNothing`,
  `TestApplyModel_eventSequence` (PlanResolved → BackupTaken* → steps →
  SessionEnded renders the progress list correctly),
  `TestApplyModel_outputCoalescing` (ring + tick: a burst of StepOutput
  renders once per tick, bounded memory),
  `TestApplyModel_stepStates` (pending/running/done/failed/sub-step rows,
  NeedsTTY marker),
  `TestApplyModel_cancelNamesStep` (cancelled screen carries the
  interrupted step + resume hint, contract 2),
  `TestApplyModel_alreadyRunningRefusal` (renders the typed error, no
  second session),
  `TestHandover_ExecProcess` (Update returns `tea.ExecProcess` on the
  handover message; alt-screen restored after; the session goroutine's
  `done` releases).
- E2E (fake child processes): a NeedsTTY step actually receives the
  terminal under `tea.ExecProcess` with stdio wired; cancel mid-step
  group-kills children including a handover child.

# Implementation notes

- Events pump with `p.Send` off one drain goroutine; the run handle's
  mandate — events MUST be drained — is owned here once and for all
  windows the TUI opens.
- Output coalescing (ring buffer + tick, research 0059 §2) is TUI-
  internal; raw line events are never rendered one-per-frame. Event-mode
  chunks are colorless — the pane's own styles do the coloring.
- Handover: `p.Send(handoverMsg)` → `Update` returns
  `tea.ExecProcess(cmd, done)` with stdio wired by the TUI (nil-stdio
  session-built cmd, 0064-D9 — never pre-wired pipes); `ApplyOpts.
  HandoverAvailable = true` — the TUI can always hand the terminal over.
- Pane-run (non-TTY) steps keep `--yes` semantics via the session (the
  null-stdin no-op trap, issue 0028, is the session's problem, already
  solved).
- The apply mode replaces the main pane (0062-D4); `esc` does not pop a
  running apply — cancel is the explicit exit (gated confirm).

# Docs

- tui.md apply section (drift fixes only); README tui row blurb; log
  entry; map 0057 destination check (the design set is now *built* —
  implementation continues as ordinary work beyond M14).

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
