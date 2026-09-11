---
type: Issue
title: Apply session &amp; TTY suspend design
description: Design the apply-session shape (events, progress, cancel, terminal handoff) the service layer carries so apply streams inside the TUI.
tags: [wayfinder, grilling, apply, api]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0064: Apply session &amp; TTY suspend design

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariants 2, 11, 12, 13](../product/contract.md), [0061 service-layer architecture](0061-service-layer-architecture-cli-migration.md)
- **Related code**: [`cmd/apply.go`](../../cmd/apply.go), [`internal/mise/`](../../internal/mise/), [`internal/state/`](../../internal/state/), [`internal/apply/`](../../internal/apply/), [`internal/executil/`](../../internal/executil/)
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

Round 2 settled the residuals D1–D6 leave open.

- **D7 Result & error set**: exactly two additions to 0061-D3's typed
  taxonomy — `AlreadyRunningError` (D6) and
  `SessionCancelledError{StepName}` — in the service error area,
  `errors.As`-able like their siblings. `Wait` returns
  `SessionResult{Outcome Completed|Failed|Cancelled, StepError
  *StepError, FinalCursor string, Backups []string}` on any completed
  decision; a failed step is a *result*, not a Go error — `Wait` errors
  only on API misuse or the ctx dying before `Start` completes. A
  generic Kind-enum `SessionError` was rejected: it breaks the D3
  convention the TUI's rendering switches on.
- **D8 `ApplyOpts` field list**: `ProfilePath`, `StatePath`,
  `Modules []string`, `Sections map[string]bool` (nil = all; the CLI
  adapter translates section flags into it), `Yes`, `Force`, `Backup`,
  plus the consumer seams `Handover func(*exec.Cmd) error` and
  `Output io.Writer`. Deliberately absent: `Diff` (CLI-side reads call,
  D6) and `Verbose` (renderer concern in the CLI adapter). The kong
  flag struct never enters the service layer — flag→struct translation
  is exactly the adapter work 0061-D7 scoped.
- **D9 `Handover` stdio contract**: the session constructs the
  `exec.Cmd` completely — `Path`, `Args`, `Env`, `Dir`,
  `Setpgid: true` (D5) — with `Stdin`/`Stdout`/`Stderr` **nil**; the
  contract requires the consumer to wire stdio before running and run
  synchronously (return = step over). TUI impl: `p.Send(handoverMsg)`
  → `Update` returns `tea.ExecProcess(cmd, done)`, `done` releasing the
  blocked session goroutine; alt-screen suspend/restore is bubbletea's
  job. CLI impl: wire `os.Stdin/os.Stdout/os.Stderr` and run via the
  `StreamLive` path, carrying byte-identical output over. Pre-wired
  pipes rejected — they strip the TTY from the child and defeat
  handover's purpose. `Handover` returning an error fails the step as a
  resumable `*StepError` (cursor untouched) — a refused handover
  behaves exactly like a crashed step.
- **D10 Rendering ownership & closure**: the session's contract ends at
  "deliver events, accept cancel, expose `Wait`". The apply view-model
  (progress list, output pane, per-step diffs, cancelled-state screen)
  and output coalescing (research 0059 §2's ring + tick) are TUI-
  internal — 0065/0067 concerns, never session behavior. No new fog
  items; 0062's named deferrals stand.

## Design: the apply-session API

Sketch (normative intent, not literal code — the Go service layer is
the contract per 0060):

```go
// internal/service — session area

type ApplyOpts struct {
    ProfilePath string
    StatePath   string
    Modules     []string          // nil = all
    Sections    map[string]bool   // nil = all; CLI adapter maps flags
    Yes, Force, Backup bool
    Handover func(*exec.Cmd) error // required; session-built cmd, consumer-wired stdio
    Output   io.Writer             // nil = event mode; attached = fd passthrough
}

type ApplySession struct{ /* ... */ }

func (a *ApplyArea) Start(ctx context.Context, opts ApplyOpts) (*ApplySession, error)
func (s *ApplySession) Preview() []StepPreview // NeedsTTY+Reason per step, stable after Start
func (s *ApplySession) Events() <-chan Event
func (s *ApplySession) Cancel()                // idempotent; safe after completion
func (s *ApplySession) Wait() (*SessionResult, error)

type SessionResult struct {
    Outcome     SessionOutcome // Completed | Failed | Cancelled
    StepError   *StepError     // Failed only
    FinalCursor string         // "" when Completed (state file removed)
    Backups     []string       // when Backup ran
}

// Event set (D3, concrete types behind an Event marker interface):
//   PlanResolved{Plan, Cursor string, CursorEffective bool}
//   BackupTaken{Dir string, Files []string}
//   StepStarted{Name string, Index, Total int, NeedsTTY bool, Sub *SubStep}
//   StepOutput{Name string, Chunk []byte}          // event mode only (D2)
//   StepFinished{Name string}
//   StepFailed{Name string, Err *StepError}
//   SessionEnded{Outcome, Err *StepError, ResumeCursor string}

// Errors (service area, alongside 0061-D3's set):
//   AlreadyRunningError{}                    // Start, from state.TryLock
//   SessionCancelledError{StepName string}   // Wait, when Cancelled
```

Event sequence:

```
Start:    resolve plan → state.TryLock → classify (Preview frozen)
            ├─ typed error: PlanError / ResolveError / AlreadyRunningError
            └─ ok: goroutine starts; PlanResolved emitted (cursor + effectiveness)

run:      [BackupTaken] → per step:
            StepStarted → (StepOutput*) → StepFinished
            └ TTY step: Handover(cmd) blocks the session goroutine until
              the consumer's exec returns; NeedsTTY was already announced
          → SessionEnded{Completed} → state file removed

fail:     StepFailed{StepError} → SessionEnded{Failed, cursor = last done}
cancel:   ctx cancel → SIGKILL process group → SessionEnded{Cancelled, step}
```

`cmd/apply.go` absorption map (everything moves; nothing forks):

| today in `cmd/apply.go` | after, in the session |
|---|---|
| section flags → `buildSteps` filters | `ApplyOpts.Sections` → session-internal pipeline construction |
| D8a full-config snapshot pre-step | session pre-step (unchanged semantics) |
| `backupCopyTargets` | session pre-step → `BackupTaken` event |
| cursor save-per-step / remove-on-complete / fingerprint reset | session-owned state writes (contract 2 unchanged) |
| `printPlan` | CLI renders `PlanResolved` via the 0061-D5 canonical renderer |
| `--diff` | CLI-side reads call before `Start` (D6) |
| `stdinIsTerminal()` at config-write | handover-available flag (D4): CLI passes its own reality, TUI passes true |
| `executil.StreamLive` wiring | `Output`-attached passthrough policy (D2) |
| `state.TryLock` around `Run` | `Start`, `AlreadyRunningError` (D6) |

Invariants honored: contract 2 (cursor names the last completed step
through fail and cancel), 11 (single-flight lock at `Start`; reads stay
lock-free), 12 (post-hooks re-run on resume — the session adds no
skip logic), 13 (mise fails loud; the session never parses mise output
for semantics — research 0059 §3). CLI output parity during the 0061-D7
migration slice falls out of D2 + canonical renderers, not a forked
path.
