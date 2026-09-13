---
type: Specification
title: Service API
description: The Go service layer (internal/service) — the normative product API every front end consumes.
tags: [product, api, service]
timestamp: 2026-09-12T00:00:00Z
---

# The contract is the code

There is no IDL (issue 0060): the Go service layer at `internal/service` —
its exported interfaces, structs, methods, event types, and errors — **is**
the normative API contract for dotdrift's whole product surface. This
document describes that surface in prose for reviewers and consumers. Where
this document and the code disagree, the code wins and the document is a bug
(fix it in the same change that changes the code).

Versioning is ordinary Go package versioning (issue 0061-D1): in-module
consumers ride the module's releases; the breaking-change escape hatch is
the standard copy-forward to `internal/service/v2/`. There is no version
directory today.

# Consumers

- **The CLI** (`cmd/`) — kong stays the parser; each command is a thin
  adapter that translates flags into option structs, calls the service,
  and renders. `cmd` holds zero orchestration, zero lock logic, and zero
  domain logic.
- **The TUI** (`dotdrift tui`, milestone M14) — pumps apply events into
  bubbletea with `p.Send`, hands the terminal over with `tea.ExecProcess`.
- Any future consumer defines the narrow interface it needs and fakes that
  in tests — the service layer ships **no** interface hierarchy for
  consumers to implement.
- Non-Go consumers are deferred until one exists; if one materializes, an
  exchange format is derived from the Go types *then* (0060).

# Shape

One package, per-area service structs composed into a root the front ends
hold (0061-D2/D6):

```go
type Service struct {
    Apply *ApplyArea
    Reads *ReadsArea
    // Writes areas as migration lands them (below)
}

func New() *Service
```

| Area | Covers | Status |
|---|---|---|
| Session (apply) | apply runs: start, stream, handover, cancel, resume | **shipped** (issues 0069/0070/0071) |
| Reads | modules, plan, status, drift/diff, detect, module config at a layer dir (`ReadsArea`: `Modules`, `Plan`, `Status`, `Diff`, `Detect`, `ModuleConfigAt`; canonical renderers + `ModuleLayers`) | **shipped** (T-tui-reads; each slice byte-identical in CLI output; `ModuleConfigAt` added for T-tui-shell) |
| Writes | onboard, restore, generate, profile editing (config area: `ReadModuleLayer`/`WriteModuleLayer` + module-management ops); `WritesArea`: `Onboard`, `Restore`/`RestorePlan`, `GenerateSelection`, `Volumes`, `WriteGenerate`, plus `IndexBackups`/`NormalizeRestoreTarget`/`ModuleRel` for the browse views | **shipped** (T-tui-writes; onboard/restore/generate byte-identical in CLI output; restore's elevated copies run through `RestoreOpts.Handover` — the 0064-D9 seam) |

The layer **wraps** the domain packages and **absorbs the orchestration**
(0061-D6): the ~17 `internal/` packages keep their deep-module boundaries
and public APIs untouched; what lives in the service layer is the
coordination logic that used to live in `cmd/` (plan assembly, apply step
wiring, session state) plus error translation at its boundary.

# Reads area

`ReadsArea` is the lock-free half (reads never take the state lock,
contract 11). Each method wraps the domain preamble it is named after and
returns the typed model the domain packages already produce:

```go
func (r *ReadsArea) Modules(profilePath string, modules []string) (*ModulesRead, error)
func (r *ReadsArea) Plan(profilePath string, modules []string, f *facts.Facts) (*PlanRead, error)
func (r *ReadsArea) Status(ctx context.Context, opts StatusOpts) (*StatusRead, error)
func (r *ReadsArea) Diff(plan *resolve.Plan, profileRoot string) ([]DiffEntry, error)
func (r *ReadsArea) Detect() (*facts.Facts, error)
func (r *ReadsArea) ModuleConfigAt(dir string) (*profile.ModuleConfig, error)
```

- `Modules` runs detect → load → filter (no warns — the modules listing is
  the canonical surfacing of skips); `Plan` runs detect (when `f` is nil)
  → load → warn → filter → resolve, the exact preambles the CLI commands
  ran before the migration. `WarnLoad` (a `ReadsDeps` hook) fires between
  load and filter so the consumer's zerolog nudges keep their place.
- `Status` adds the state load (`StatePath` "" = the profile's default),
  the drift check over caller-prepared probes (`StatusOpts.ProbesFor` is
  called with the detected facts — elevation and backend/mise seams stay
  the consumer's), profile-content orphans, and the other configured
  accounts (`StatusRead.Others`). Drift is model output, never an error.
- `Diff` collects differing copy-mode dotfiles as content pairs;
  unreadable sources and missing targets are skipped (nothing to diff).
  `ModuleConfigAt` loads one layer's module.toml (a tree origin's raw
  declaration — the TUI's raw view and, later, the editors' input);
  missing declarations are `(nil, nil)`, broken files surface as
  `*SchemaError`. `ModuleLayers` exposes the every-layer module scan (shared with
  restore's backup index).
- `ReadsDeps` carries the seams (detect/load/resolve, `OtherAccounts`,
  `WarnLoad`); zero values fall back to the real implementations via
  `WithDefaults`, composing exactly like `ApplyDeps`.

# Writes area

`WritesArea` owns the write orchestration the CLI adapters used to carry:
onboard, restore, and generate run here once, and the front ends
translate input and render output. The CLI's bytes are the area's
contract — report lines go to the caller's `Out` writer (nil discards;
the CLI passes stdout, the TUI a buffer).

- `Onboard(OnboardOpts)` adopts live paths and applies: packages parse at
  the boundary (`parse packages:` wrap), detect fills an overlay owner a
  bare flag left empty, and the onboard flow runs unchanged. `DryRun` is
  an option field.
- `Restore(RestoreOpts)` and `RestorePlan(RestoreOpts)` share one
  resolution: normalize targets, index the mirrored backup layout, pin
  the newest generation or the caller's, and refuse ambiguity with the
  CLI's exact errors. Restore copies user-writable targets directly and
  runs everything else through `RestoreOpts.Handover func(*exec.Cmd)`
  (the 0064-D9 seam — the service builds the `sudo install`/`sudo rm`
  children, the consumer owns the terminal; the TUI skips elevated
  targets and names the CLI command until a terminal bridge exists).
- `IndexBackups`, `NormalizeRestoreTarget`, and `ModuleRel` are exported
  for the `restore --list` browse rendering, which stays CLI
  presentation over service data.
- `GenerateSelection`, `Volumes`, and `WriteGenerate` are the generate
  flow: selection fills owners from facts, volumes classify with the
  managed-source annotation, and the write takes an already-assembled
  `generate.Input` — assembly (flags or dialog) stays with the front
  end through the shared builders (contract 15: one assembly path).

`WritesDeps` carries the seams (detect, `LoadProfile`, `NewMise`,
`HomeDir`, `ListVolumes`); `WithDefaults` fills the real implementations.

# Session area — apply

One apply is one session: a run handle over a closed event vocabulary.
This is the only writer of convergence state; reads never take the lock
(contract 11).

## Starting

```go
func (a *ApplyArea) Start(ctx context.Context, opts ApplyOpts) (*ApplySession, error)
```

`Start` resolves the plan, takes the sidecar state lock non-blocking
(another running apply is `*AlreadyRunningError`, never a queue), freezes
the per-step TTY classification, and launches the run goroutine. It
returns once the session is armed; events flow from the goroutine, so the
consumer must drain `Events` concurrently (the run blocks on send once the
channel buffer fills). Synchronous errors before the session exists: a
missing `ProfilePath` or `Handover`, detect/load/filter/resolve failures.

```go
type ApplyOpts struct {
    ProfilePath string
    StatePath   string          // "" = the profile's default state path
    Modules     []string        // nil = all
    Sections    map[string]bool // nil/empty = all; the CLI adapter maps section flags
    Yes         bool
    Force       bool
    Backup      bool
    Handover    func(*exec.Cmd) error // required; see "Terminal handover"
    Output      io.Writer             // nil = event mode; attached = fd passthrough
    HandoverAvailable *bool           // nil = the process's stdin reality; a UI that can hand the terminal over sets true
}
```

The kong flag struct never crosses the boundary — flag→struct translation
is exactly the adapter's job (0064-D8). `--diff` stays a CLI-side reads
call *before* `Start`; `--verbose` is a rendering concern of the CLI
adapter, not an option.

## The event stream

```go
func (s *ApplySession) Events() <-chan Event  // closed after SessionEnded
func (s *ApplySession) Preview() []StepPreview // frozen at Start; stable
func (s *ApplySession) Cancel()                // idempotent; no-op after the end
func (s *ApplySession) Wait() (*SessionResult, error)
```

`Event` is a closed vocabulary (0064-D3) — consumers type-switch the
concrete types, and semantics come from step boundaries, hook indexes and
exit codes, **never** from parsing mise output (contract 13):

| Event | Meaning |
|---|---|
| `PlanResolved{Plan, Profile, Facts, Cursor, CursorEffective}` | opens the stream; carries the loaded resume cursor and whether it is effective for this selection (a cursor naming a step absent from this run is stale and ignored — contract 2) |
| `BackupTaken{Dir, Files, Count}` | one per receiving module dir when `Backup` is set; `Count` is destinations actually written |
| `StepStarted{Name, Index, Total, NeedsTTY, Sub}` | a step began; hooks fire one per command with `Sub` set |
| `StepOutput{Name, Chunk}` | one line of child output — **event mode only**; colorless by construction |
| `StepFinished{Name}` | step completed, cursor advanced |
| `StepFailed{Name, Err, Sub}` | `Sub nil` ends the session Failed right after; a `Sub`-carrying failure reports one hook command (required hooks precede the step-level failure, optional ones continue) |
| `SessionEnded{Outcome, Err, ResumeCursor}` | closes the stream; `ResumeCursor` is `""` on Completed (state file removed), otherwise names the last completed step |

Sequence: `PlanResolved` → `BackupTaken*` → per step
`StepStarted` → (`StepOutput`*) → `StepFinished` | `StepFailed` →
`SessionEnded`. A TTY step's `Handover` happens synchronously inside the
run goroutine between its `StepStarted` and `StepFinished`; `NeedsTTY`
let the UI announce the takeover first.

## Output policies

Two consumption policies, one pipeline (0064-D2), picked by `ApplyOpts.Output`:

- **Attached** (the CLI): children stream fd-direct to the writer —
  terminal color survives, output is byte-identical with the pre-service
  CLI; no `StepOutput` events are emitted. The child's stderr stays on the
  process stderr so mise warnings and the verbose echo do not migrate into
  the stdout stream.
- **Absent** (the TUI): child output is line-buffered into colorless
  `StepOutput` events. Pane-run steps keep `--yes` semantics — a null
  stdin makes mise silently no-op while exiting 0 (issue 0028's trap).

## Terminal handover

`Handover` is the seam that lets a step that truly needs the terminal
(sudo prompts, `interactive = true` hooks, TTY-gated probes like
`smbpasswd`) run under the consumer's real terminal, without the service
importing any UI package (0061-D4, 0064-D9):

- The session constructs the `exec.Cmd` completely — path, argv, env, dir,
  process group set for group-kill on cancel — with **stdio nil**. The
  consumer wires stdio before running (pre-wired pipes are forbidden:
  they strip the child's TTY and defeat the handover).
- The consumer runs the command **synchronously**; its return ends the
  handover. TUI: `tea.ExecProcess` (alt-screen suspend/restore is
  bubbletea's job). CLI: wire the real fds and run.
- A `Handover` error fails the step as a resumable `*StepError` — a
  refused handover behaves exactly like a crashed step.
- Which steps will hand over is queryable up front: `Preview()` returns
  `{Name, NeedsTTY, Reason}` per step, frozen at `Start`, so a gate can
  say "N steps will take the terminal" before anything runs. The
  `interactive = true` opt-in at config-write keys on **handover
  availability** (`HandoverAvailable`), not raw stdin state: the CLI
  passes its own stdin reality, a UI that can hand the terminal over
  passes true.

## Cancel and results

`Cancel` kills the process group immediately (children mid-handover
included) — installs run minutes, so finish-then-stop would lie (0064-D5).
The pipeline returns without advancing the cursor past the interrupted
step, so the resume cursor names the last completed step (contract 2);
backups already taken stay.

```go
type SessionResult struct {
    Outcome     SessionOutcome // Completed | Failed | Cancelled
    StepError   *StepError     // Failed only
    FinalCursor string         // "" when Completed
    Backups     []string       // backup generation dirs, when Backup ran
}
```

A failed step is a **result, not a Go error** — `Wait` errors only with
`*SessionCancelledError` when the outcome is Cancelled. A cancel racing a
step that failed on its own merits never rewrites the outcome the
`StepFailed` event announced.

# Error taxonomy

Exported typed errors live in `internal/service`, which translates domain
errors at its boundary; front ends match with `errors.Is`/`errors.As` and
never parse strings (0061-D3):

- `StepError{Step, Err}` — a step failure; always resumable (contract 2).
- `AlreadyRunningError{StatePath}` — `Start` refused: another apply holds
  the sidecar lock (contract 11).
- `SessionCancelledError{StepName}` — `Wait`'s error return on a cancelled
  session; names the interrupted step.
- `SchemaError{Path, Line, …}` — strict-schema load failures (shipped with
  the reads area; the TUI renders it and opens a broken file read-only,
  0065-D8). Every read translates profile load errors at its boundary;
  `Error()` carries the original string so CLI output is byte-identical.

# Rendering ownership

Canonical text renderers live in the service layer **only** for surfaces
both front ends show fact-identically: the plan report, diff, and status
summary (0061-D5) — shipped as `RenderPlanReport`, `RenderDiff`, and
`RenderStatusHeader`/`RenderStatusNote` (composed by
`RenderStatusSummary`; the CLI interleaves its `--diff` section between
them). `--json` is a dumb marshal in `cmd/`; all TUI-styled rendering
stays TUI-side and never shares code with the canonical renderers. CLI
output must stay byte-identical across every migration slice — that gate
is what makes the big-bang migration safe (0061-D7), and the service
renderers are pinned to the pre-migration bytes under
`internal/service/testdata/golden`.

# Rules for change

- The event vocabulary is closed: a new event is a new concrete type plus
  a same-change update of this document. Semantics never arrive by parsing
  child output (contract 13).
- New failure modes are typed errors in the taxonomy above, matchable with
  `errors.Is`/`errors.As` — no wrapped-string-only errors at the boundary.
- The section vocabulary is single-sourced in the service
  (`ResolveSections`); flags, filters, and UIs translate into
  `ApplyOpts.Sections`, never into step lists.
- Reads are lock-free; only the session writes convergence state and only
  it holds the lock.
- `internal/service` imports no UI package, and no consumer's flag struct
  crosses the boundary.
- Every orchestration fact lives in exactly one place — the session — not
  once per front end.

# References

Issues [0060](../issues/0060-idl-choice-go-derivation.md) (no IDL),
[0061](../issues/0061-service-layer-architecture-cli-migration.md)
(architecture + migration), [0064](../issues/0064-apply-session-tty-suspend-design.md)
(session design), [0065](../issues/0065-full-schema-editor-suite-design.md)
(config area), [0069](../issues/0069-implement-apply-session-service-core.md)/[0070](../issues/0070-migrate-cmd-apply-onto-apply-session.md)/[0071](../issues/0071-surface-tty-handover-for-real-steps.md)
(implementation); ADR-0008 (the doorway); [CLI surface](cli-surface.md);[package layout](../engineering/package-layout.md).
