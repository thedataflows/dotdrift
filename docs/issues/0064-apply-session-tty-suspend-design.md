---
type: Issue
title: Apply session &amp; TTY suspend design
description: Design the apply-session shape (events, progress, cancel, terminal handoff) the service layer carries so apply streams inside the TUI.
tags: [wayfinder, grilling, apply, api]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0064: Apply session &amp; TTY suspend design

- **Type**: task
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri (main session)
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

## Decisions

Round 1 grounded in [research 0059](../research/0059-apply-streaming-tty-precedents.md),
[0061 D3/D4/D6/D8](0061-service-layer-architecture-cli-migration.md), and the
orchestration in `cmd/apply.go` (`buildSteps` → `apply.Pipeline` → cursor
save/remove, D8a snapshot, `--diff`/`--backup` pre-steps).

- **D1 Session API shape**: run handle + event channel —
  `svc.Apply.Start(ctx, ApplyOpts) (*ApplySession, error)`; the session
  runs the pipeline on its own goroutine and exposes
  `Events() <-chan Event`, `Cancel()`, `Wait() (*SessionResult, error)`.
  The TUI pumps events with `p.Send`; the CLI drains the channel inline.
  Blocking handler interfaces rejected: the TUI needs an event bridge
  anyway and cancel gets a first-class method. The `Handover`
  callback (0061-D4) stays synchronous inside the service goroutine.
- **D2 Output policy**: two consumption policies, one pipeline —
  `ApplyOpts.Output io.Writer`. When attached (CLI), child fds wire
  directly as `executil.StreamLive` does today (child color survives;
  byte-identical per 0061-D7) and no `StepOutput` events are emitted;
  when absent (TUI), child output is line-buffered into `StepOutput`
  events (colorless — the honest default). CLI renders `PlanResolved` /
  backup events through 0061-D5's canonical renderers. No tee, no
  forked CLI path.
- **D3 Event vocabulary**: closed set of concrete Go types —
  `PlanResolved{Plan}` → `BackupTaken{Files, Dir}` (when enabled) → per
  step `StepStarted{Name, Index, Total, NeedsTTY, Sub *SubStep}` →
  `StepOutput{Name, Chunk}` → `StepFinished{Name}` / `StepFailed{Name,
  *StepError}` → `SessionEnded{Outcome, *StepError, ResumeCursor}`.
  Hooks surface as sub-steps `{Index, Total, Command}` on the hooks
  steps (they run as individual mise tasks today). The TTY handover is
  **not** an event — it is the synchronous 0061-D4 `Handover(*exec.Cmd)
  error` callback; `StepStarted.NeedsTTY` lets the UI announce the
  takeover first. Semantics from step boundaries, hook indexes and exit
  codes — never from parsing mise output (research 0059 §3).
- **D4 TTY classification & interactive hooks**: the session exposes a
  preview query `Session.Preview() []StepPreview{Name, NeedsTTY,
  Reason}` computed at start from plan + facts (euid, writability,
  `interactive = true` hooks, per-file prompts), so the plan-view gate
  can say "N steps will take the terminal" before running. The
  `interactive = true` opt-in at config-write keys on **handover
  available** — CLI: its real stdin (today's `stdinIsTerminal`), TUI:
  always true — not raw stdin state. Pane-run steps keep `--yes`
  (research 0059 §2: null stdin makes mise silently no-op while exiting
  0 — issue 0028's trap).
- **D5 Cancel semantics**: immediate process-group kill — one
  cancellable ctx from the consumer through the service to
  `exec.CommandContext` with `Setpgid` + group SIGKILL on every child
  path, including the streaming `runOp` path (research 0059 §5 flags it
  as lacking `Setpgid` today) and children mid-handover (sudo included).
  The pipeline returns without saving the failed step, so the resume
  cursor names the last completed step (contract 2); outcome is
  `Cancelled` with the step name; backups already taken stay. Finish-
  step-then-stop rejected: installs run minutes and "cancel" would lie.
- **D6 Lifecycle, lock & cursor ownership**: `Start` = resolve plan →
  `state.TryLock` (typed `AlreadyRunningError`, 0061-D8; reads stay
  lock-free) → classify → emit `PlanResolved` (carrying the loaded
  resume cursor and whether it is effective for this selection —
  stale-cursor ignore rule unchanged, contract 2) → run. The session
  owns cursor save-after-each-step, state-file removal on completion,
  and the D8a full-config snapshot; `cmd/apply.go`'s orchestration
  (`buildSteps`, config paths, backup pre-step) is absorbed per
  0061-D6. `--diff` preview stays CLI-side as a reads call before
  `Start`.
