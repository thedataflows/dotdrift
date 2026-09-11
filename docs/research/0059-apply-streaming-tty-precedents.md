---
type: Reference
title: Apply streaming & TTY precedents
description: bubbletea's terminal-handover and streaming mechanics, real-TUI precedents, and what dotdrift's apply pipeline already guarantees — facts for the apply-session design (0064).
tags: [research, tui, apply, tty]
timestamp: 2026-09-11T00:00:00Z
---

# Research 0059: Apply streaming & TTY precedents

Grounded against bubbletea **v1.3.6** — the version dotdrift already pins
(indirect via huh v1.0.0, `go.mod`) — and against this repo's
`cmd/apply.go` / `internal/mise` / `internal/apply` / `internal/state`.

## 1. bubbletea's official terminal-handover mechanisms (v1.3.6)

- **`tea.ExecProcess(c *exec.Cmd, fn)`** ([exec.go](https://github.com/charmbracelet/bubbletea/blob/v1.3.6/exec.go))
  is *the* tool for steps that need a real terminal. Returned as a
  `tea.Cmd`; when the event loop receives it, it **blocks the event loop**:
  `p.ReleaseTerminal()` → wires the child's stdio (stdin = the program's
  input, stdout = the program's output, stderr = `os.Stderr`) → `c.Run()`
  → renderer reset → `p.RestoreTerminal()` → `p.Send(fn(err))`. The child
  runs on the real terminal with termios/alt-screen state fully restored,
  so `sudo` prompts and mise's interactive hooks work; no Update/render
  happens while it runs. Stdio set on the `exec.Cmd` beforehand is
  preserved (the setters only fill unset fields), so `cmd.Dir`, `Env`, and
  argv stay fully dotdrift-controlled. Official example:
  [examples/exec/main.go](https://github.com/charmbracelet/bubbletea/blob/v1.3.6/examples/exec/main.go)
  (open `$EDITOR` on a keypress).
- **`tea.Exec(ExecCommand, fn)`** generalizes the above to any type with
  `Run/SetStdin/SetStdout/SetStderr` — the extension point if a step ever
  needs a custom handover (e.g. run the child inside a PTY while the rest
  of the screen stays ours).
- **`p.ReleaseTerminal()` / `p.RestoreTerminal()`**
  ([tea.go](https://github.com/charmbracelet/bubbletea/blob/v1.3.6/tea.go))
  are the manual pair underneath `ExecProcess`, for arbitrary blocking code
  that isn't an `exec.Cmd`. Release: ignores signals, cancels+drains the
  input reader, stops the renderer, remembers alt-screen/bracketed-paste/
  focus state, restores termios. Restore: re-enters raw mode, restarts the
  reader and renderer, re-enters alt screen (or sends repaint), and calls
  `checkResize()` — **resizes that happened during the handover are picked
  up here**. Both are plain `*Program` methods, callable from a `tea.Cmd`
  goroutine or the service layer.
- **`tea.Suspend()`/`SuspendMsg`** is job-control suspension (ctrl+z →
  SIGTSTP, `ResumeMsg` on fg). Not a subprocess mechanism; don't confuse
  the two.
- **`tea.ExecFramebuffer` is not real** — zero symbols on
  [pkg.go.dev](https://pkg.go.dev/search?q=ExecFramebuffer), absent from
  v1.3.6 and from main. The ticket's "(if real)" resolves to *no*; nothing
  in any bubbletea line renders a child through the TUI framebuffer. The
  closest real thing is a custom `ExecCommand` under `tea.Exec`.
- Related shutdown facts (tea.go): `tea.WithContext(ctx)` makes a cancelled
  ctx abort `Run` with `ErrProgramKilled`; `p.Kill()` skips the final
  render; `p.Send` is a safe no-op after exit. **`tea.Cmd`s are never
  cancelled** — the commands goroutine is deliberately abandoned ("It's
  not possible to cancel them"), so a long-running child must ride its own
  `exec.CommandContext`, not bubbletea's context. Note also `Program.Println/
  Printf` print *above* the program but are **suppressed while the alt
  screen is active** — useless for a full-window log pane; pane content
  must be model state.

## 2. Streaming a long-running subprocess into a model

- Canonical shape: a `tea.Cmd` runs/reads the child and returns messages;
  each `Update` appends to model state; `View` renders it.
  `tea.Batch` fans commands out concurrently, `tea.Sequence` serializes
  them (both visible in the eventLoop source). Any goroutine can
  `p.Send(msg)` from outside.
- Backpressure is a real constraint: `Program.msgs` is an **unbuffered**
  channel and the event loop processes exactly one message per
  update+render cycle. Per-line (let alone per-byte) messages starve
  rendering under chatty children. The proven mitigation is coalescing:
  the reader goroutine appends into a bounded buffer (ring of lines) and a
  `tea.Tick` (~100 ms) drains it into the model, decoupling child speed
  from frame rate. The render pane is a scrollable log (bubbles
  `viewport`, already an indirect dep) that follows the tail (keep
  `GotoBottom` only while the user is at the bottom).
- Color: children only keep their ANSI when stdout is a *terminal*. This
  repo verified the converse for our children — mise/paru "emit zero ANSI
  when their output is a pipe and honor no force-color override"
  (`internal/mise/mise.go`, `runOp` comment). So a pipe-fed stream pane is
  colorless unless the child is given a PTY (§3).

## 3. PTY options

[creack/pty](https://github.com/creack/pty) (README): `pty.Start(c)` gives
the child a pseudo-terminal slave and returns the master; `pty.InheritSize`
propagates the window size on SIGWINCH; the parent reads the master into
the model. This makes `isatty` true for the child (color, spinners) while
the output is captured — lazygit's per-view PTYs exist exactly for this
(§4). Two caveats that matter for us:

1. An app-made PTY is **not the controlling terminal** unless you set
   `Setsid`/`Ctty` via `pty.StartWithAttrs`; `sudo` opens `/dev/tty`, so a
   sudo password prompt does not reliably work through a spawned PTY. For
   sudo prompts the robust answer is handing over the *real* terminal —
   `tea.ExecProcess` (§1).
2. PTY output carries the child's cursor-control sequences, so it is
   display-only; never parse it for semantics (also dotdrift's existing
   stance, §5). And the child process tree must be killed/reaped on
   cancellation — closing the master only stops the reader.

## 4. Precedents

- **lazygit** (gocui, but the architecture maps 1:1) —
  [pkg/gui/gui.go](https://github.com/jesseduffield/lazygit/blob/master/pkg/gui/gui.go):
  - Interactive/privileged commands: `runSubprocessWithSuspense` — take a
    `SubprocessMutex`, `gui.suspend()` (tear down the UI, restore the
    terminal, pause background refreshes), wire the subprocess's stdio to
    `os.Stdin/Stdout/Stderr`, echo `+ argv`, block on `Run()`, optional
    "press enter to return" prompt, then `gui.resume()`. That is
    `tea.ExecProcess` in all but name — suspend/handover/resume, never
    interleaving child output with the UI.
  - Streaming panes: a **per-view `ViewBufferManager`** decouples command
    output from the render loop, and `viewPtmxMap` keeps "a mapping of
    view names to ptmx's … for rendering command outputs from within a
    pty", resized with the window. Lazy-loaded views read ahead 3
    screenfuls when scrolling (`scrollReadAheadScreenfuls`).
  - Cancellation: on exit every buffer manager is closed and
    `oscommands.TerminateLivePtys()` reaps **process trees** "so that they
    don't outlive lazygit" — kill the group, not the direct child.
- **gh-dash** (bubbletea itself) —
  [internal/tui/tasks.go](https://github.com/dlvhdr/gh-dash/blob/main/internal/tui/tasks.go),
  [context.go](https://github.com/dlvhdr/gh-dash/blob/main/internal/tui/context/context.go):
  progress is a **task registry, not streamed output**: `Task{Id,
  StartText, FinishedText, State: TaskStart|TaskFinished|TaskError,
  Error}`, `StartTask(task) tea.Cmd`, the work `tea.Cmd` returns
  `TaskFinishedMsg{TaskId, Err}`, a spinner status line shows running
  tasks. Custom commands run via `$SHELL -c` (internal/shell). Telling
  detail: launcher subprocesses get `io.Discard` "so any noise … does not
  leak into the TUI's terminal and corrupt the display" (#829/#584/#679).
  gh-dash streams nothing into panes — for bubbletea, "start/finish/error
  task + spinner" is the battle-tested minimal progress model.

## 5. What dotdrift's apply pipeline already guarantees

- **Step model**: `apply.Step` is `Name() + Run(ctx)`
  (`internal/apply/apply.go`); the pipeline skips through the cursor
  (`state.LastCompleted`), saving after every step, deleting the file on
  completion; a cursor naming a step absent from this run is ignored
  (stale/other-selection). Step set and order (`cmd/apply.go buildSteps`):
  `hooks-pre → packages → tools → dotfiles → systemd → dotfiles-system →
  mounts → smb → hooks-post`, each conditional. A TUI progress model keys
  off these 9 names + per-hook index (`HooksStep` runs each command as its
  own `mise run <task>`), plus exit codes — never off parsing mise's
  output. This matches the repo's existing stance: steps decide from
  pre-computed facts, not stderr (`systemFilesStep` pre-checks target
  writability precisely *because* `runOp` streams and "a 'Permission
  denied' can't be read back from stderr").
- **Which steps need a TTY is pre-computable** — the session can classify
  before running anything:
  - *sudo prompts*: the elevated-edits step runs `sudo -E mise dotfiles
    apply` iff `euid != 0 && !systemTargetsUserWritable(edit targets)`
    (`cmd/apply.go`, `internal/mise/mise.go dotfilesApplyArgv`); mise's
    bootstrap privileged batch "prompts through sudo only on a TTY, and
    otherwise fails loud with the exact command" (contract 13, verified
    against mise 2026.9.1) — needed iff the plan has whole-file system
    entries/mount dirs.
  - *interactive hooks*: hook tasks carry `interactive = true` (contract
    12) — declared in the plan, generated at config-write time from
    `stdinIsTerminal()` (`GenerateApplyConfig` call in `cmd/apply.go`).
    Under a TUI the decision must not naively reuse "stdin was a terminal
    at launch": the TUI owns the terminal and can always hand it over via
    `ExecProcess`, so interactive=true is safe whenever the app has a real
    terminal.
  - *per-file prompts*: mise asks `apply <target>?` before changes; on a
    null stdin it resolves "No", **exits 0, and the step is recorded
    complete with the drift persisting** (issue 0028, `opStdin` comment).
    Inside a pane, steps must run with `--yes` (`ApplyCmd.Yes` already
    plumbs through; mounts/smb hardcode yes) or behind a handover —
    "capture and answer later" is not a thing.
- **Execution reality to preserve**: every mise subprocess is spawned from
  a pinned neutral cwd + `MISE_TRUSTED_CONFIG_PATHS` (`Mise.WorkDir`,
  `trustEnv`); child color survives only when fds go straight to a
  terminal (`executil.StreamLive` wires child fds directly, no
  MultiWriter). Under bubbletea that wiring is exactly what must *not*
  happen except during a handover — the runner needs an event/tee seam
  instead of `os.Stdout`. `--verbose` additionally streams and echoes
  `+ argv`, and sets `MISE_VERBOSE=1` (mise debug lines on stderr).
- **Cancellation gap worth knowing**: `runContextEnv` sets
  `Setpgid` + ctx-Cancel = SIGKILL of the whole process group (added
  because shell-wrapped children kept pipes open), but the **streaming
  path in `runOp` sets neither** — a default `CommandContext` cancel kills
  only the direct child. `ApplyCmd.Run` currently uses
  `context.Background()`, i.e. there is no cancel path at all today. The
  apply-session should funnel one cancellable ctx through the service
  layer to `exec.CommandContext` with process-group kill everywhere
  (lazygit's `TerminateLivePtys` is the precedent for reaping).

## 6. Cancel semantics and the resume cursor

Mid-run abort (ctx cancel or child failure): `Pipeline.Run` returns
`step <name>: …` **without** saving that step, so the cursor still names
the last completed step; the next apply resumes from the failed step
(contract 2, no flag). TUI presentation falls out directly: show
"stopped at `<failed step>` — rerun to resume", and when the current
section selection makes the cursor stale, show "cursor ignored — running
all steps" (that ignore rule already exists). The sidecar flock
(contract 11) is held across the whole run today; `state.FileStore.TryLock`
already exists, so a second TUI can refuse with "another apply is running"
instead of blocking on `Lock`.

## Implications for dotdrift (for ticket 0064)

1. **Handover = `tea.ExecProcess`** for sudo-elevated steps and interactive
   hooks; `ReleaseTerminal`/`RestoreTerminal` only for non-exec blocking
   work. `ExecFramebuffer` doesn't exist; `tea.Exec`'s `ExecCommand` is the
   seam if a custom (PTY) handover is ever needed.
2. **TTY-needing steps are classifiable up front** from euid, writability
   probes, and plan declarations — the apply-session should compute a
   per-step `needs-tty` before running, instead of discovering it
   mid-stream.
3. **Progress = step boundaries + hook index + exit codes**; child output
   is display-only. Never parse streamed (let alone PTY/ANSI) mise output
   for semantics — that codifies the existing `systemFilesStep` stance.
4. **Streamed steps run unattended with `--yes`**; only classified steps
   suspend the UI. Per-file prompts answered by a null stdin silently
   skip changes while exiting 0 — a pane must never run mise that way.
5. **Runner seam**: add an event/chunk-out seam to the mise runner (tee or
   callback) replacing the "fds straight to the terminal" `StreamLive`
   behavior inside the TUI; keep pinned cwd + trust env untouched.
6. **Streaming shape**: bounded line ring + `tea.Tick` coalescing +
   bubbles viewport tail (bubbles is already a dep); colorless output is
   the honest default (mise/paru emit no ANSI on pipes) — a PTY-wrapped
   runner is the optional upgrade for color, never for semantics.
7. **Cancellation**: one ctx from the TUI through the service layer;
   `exec.CommandContext` + process-group kill on *every* child path
   (`runOp` currently lacks `Setpgid`); remember bubbletea `Cmd`s are not
   cancellable — the child's ctx is the only kill path. On abort, the
   cursor is untouched by design; the TUI just says "rerun to resume".
8. **Single-flight**: reuse the sidecar flock; use `TryLock` to show a
   friendly "apply already running" state instead of blocking.
9. **Minimal viable progress model** (gh-dash): per-step
   Start/Finished/Error task states + spinner; the output pane is a
   stretch goal layered on the same events, not a prerequisite.
