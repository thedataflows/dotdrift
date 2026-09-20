---
type: Specification
title: TUI
description: dotdrift tui — the one interactive surface; the compositor shell — nav, sectioned workspace with inline editing, modals, palette, streamed apply.
tags: [product, tui]
timestamp: 2026-09-12T00:00:00Z
---

# Command surface

`dotdrift tui [--profile PATH]` (default `.`) opens the shell at a
profile root. The TUI is dotdrift's **one interactive home**
([ADR-0007](../adr/0007-generate-cli-only.md)) — every other command is
strict flag mode, and everything interactive lives here. Apply (`P`) is
the TUI's **only** write path into convergence; profile *files* are
written only by the workspace's inline edits and the module-management
dialogs, both below.

# The shell (M15)

# The compositor (M15, landing task by task)

One shell — header, nav, workspace, footer — plus a stack of modal
layers over it (M15,
[milestone](../milestones/m15-tui-compositor.md), issue
[0073](../issues/0073-tui-compositor-redesign.md); it replaced the M14
view stack wholesale). Its parts:

- **The base shell.** Full-screen (altscreen) layout: header, nav left,
  workspace right, footer. The header names the profile root, the
  host/user context, a dirty count (`● N`) when drafts exist, and an
  apply badge while a session runs. The footer has two lines: a spinner
  with the running operation's name (or the message slot), and the key
  hints for the focused pane.
- **The message slot.** Any operation longer than ~200 ms announces
  itself (`opStartedMsg`) and reports (`opFinishedMsg`). A success note
  fades after 4 s. A failure renders in the Error color and persists
  until the next user action.
- **The modal stack.** Modals render centered over the dimmed base
  (lipgloss v2 layers on a canvas, the composited final frame). While a
  modal is open it owns all input; covered modals render nothing and
  receive nothing.
- **One esc rule.** The compositor owns `esc`, not the views: it pops the
  top modal first, then exits workspace edit mode, then returns focus to
  the nav. No per-view fallthrough.
- **Focus.** `tab`/`shift+tab` move between nav and workspace; focus is
  the border color; exactly one pane is focused.
- **Goldens** pin the composited final frames at fixed sizes (100x30 and
  64x24), ANSI stripped; message-driven tests pin focus, esc, capture,
  and chrome behavior.
- **The nav** (T-tui-nav). Module rows with overlay layers as expandable
  children: `▸`/`▾` marks a module with layers, children indent as
  `base`, `user <name>`, `host <name>` (`user root (superuser)` per
  issue 0029's visibility). `j`/`k` move, `l`/`right` expands, `h`/`left`
  collapses or jumps from a child to its module row. Selecting a layer
  child points the workspace at that layer file; the workspace and the
  nav never disagree. A module row is dirty (`●`) when any of its layer
  files has a draft in the 0065 ledger; the child names its own file's
  draft. Expansion state is remembered per session, and a reload keeps
  the selected row selected (clamping when the row is gone). Skipped or
  failed modules stay visible, greyed, naming the reason on the row;
  while the read is in flight the pane shows placeholder rows; an empty
  profile shows `(no modules)`. Rows truncate to the pane width.
- **The workspace** (T-tui-workspace). `module.toml` rendered as a
  sectioned read surface over the 0065 config seam (strict decode + raw
  text; the workspace never parses TOML): fixed-order sections — meta,
  packages, links, writes, when, hooks, systemd.units, tools, and an
  "other" group for the rest of the schema — with `(none)` for empty
  sections, never a blank pane. The title line carries the layer tabs
  (`base · user cri · host myhost`, active bracketed) in sync with the
  nav both ways: selecting a layer child moves the tab, `L` cycles the
  tabs and moves the nav cursor. A module with a superuser overlay shows
  `needs root`. A broken `module.toml` no longer kills the shell: the
  interactive load (`profile.LoadTolerant`; strict `Load` stays the
  plan/apply path) greys the module's nav row with the error, and the
  workspace opens the file read-only — the error named relative to the
  layer dir, the raw text visible, every line width-clamped. `j`/`k`
  walk entry rows (headers and empty markers are skipped) so
  T-tui-editing hangs edit mode off the same cursor.
- **Inline editing** (T-tui-editing). `enter`/`e` turns the field under
  the cursor into an input without leaving the workspace; the tier-1
  error renders at the field live. Committed edits splice through the
  profile family encoders into the file-scoped draft (0065), which
  re-decodes on every commit — a draft can never hold unparseable text.
  Dirty shows per row, on the tab line, and on the nav rows. Drafts are
  file-scoped: navigation and layer switches never prompt. `ctrl+s`
  saves through the untouched 0065 pipeline (staged tier-1 errors and
  the tier-2 cross-check block in place; a disk-hash conflict opens a
  reload-or-keep modal); `D` discards with a confirm naming module,
  layer, and change count; `a` adds a row to packages/tools/hooks/links,
  `d` removes one with confirm. A broken file edits as raw text lines
  (`e` on a line); a repair that parses unlocks the structured surface,
  and its save sends the whole repaired file as the raw candidate
  (`SaveRequest.Raw` — the splice step swaps out, every other pipeline
  check stands). Secrets, mounts, smb, and nested when groups remain
  read-only rows until their 0074 tasks land.
- **Structural editing** (issue 0074, landing task by task). The
  structural families join the inline grammar as container + field rows:
  a systemd unit is a container row (d removes with confirm, `a` adds a
  unit by name) whose directives render beneath it as indented rows;
  directive values are typed TOML — `ExecStart = /usr/bin/demo` needs no
  quoting, while `"x"`, `30`, `[a, b]`, and inline tables land with their
  TOML types.
- **The modal family** (T-tui-modals). One confirm component — question
  title, consequence body with full target identity, `y` confirms and
  every other key (or `esc`) cancels — backs dirty-quit (`q` with
  drafts), row removal, module deletion, and the destructive-apply gate.
  `m` on a module opens the 0072 manage menu as a modal (create / move /
  delete with the orphan preview), domain logic untouched. `P` runs
  apply: preview → destructive confirm when steps will overwrite
  existing files (the new `OverwriteTargets` classification on copy-mode
  dotfile steps) → the elevation modal when the plan carries privileged
  steps — one prompt listing every reason, three failures abort, `esc`
  aborts before anything is touched, the password is a zeroed `[]byte`
  fed to `sudo -k -S -v` (`executil.SudoValidate`, injected in tests),
  never drafted or logged. While a run lives the header badge and footer
  carry status; `a` opens the apply detail modal (step rows, output
  tail, verdict) — closing never cancels, `ctrl+c` inside asks first.
  Session events are compositor-level messages: they flow while any
  modal is open.
- **The palette** (T-tui-palette). `/` opens the fuzzy palette — the
  teleport path: modules with overlay layers as separate entries
  (`demo · user cri`), contextually valid actions (apply, manage, new
  module, save/discard draft — only what can run right now), and the
  current module's non-empty sections as deep links. Fixed section order
  (modules, actions, fields); ranking is sahilm/fuzzy (vendored) with an
  exact/prefix affinity tier and shorter-label tiebreak, recency
  (in-session, last 8) breaking ties last. Empty query shows recents
  then all modules; `no matches` carries the dimmed `ctrl+n` hint that
  opens module creation with the query prefilled (never auto-offered).
  Choosing a module jumps nav and workspace in sync (focus to the
  workspace); a dirty draft never prompts on a jump. esc restores the
  exact prior state. Mouse: wheel moves, click chooses.
- **The keymap** (T-tui-keymap). One binding table as data
  (`keyTable()` in keymap.go) — the base dispatcher consults it, the
  footer hints and the contextual `?` help render from it, so docs
  cannot drift from behavior. Shift is the dangerous version (`p` plan /
  `P` apply, `d` remove row / `D` discard draft); esc has exactly one
  meaning (pop the top layer: modal → edit → focus to nav); there is no
  undo — drafts and confirms are the safety net. `p` opens the read-only
  plan modal (step classification, sudo reasons, overwrite counts —
  never credentials); `w` opens the writes menu (onboard / restore /
  generate as absorbed dialogs); `n` opens module creation. Mouse parity
  on the base: wheel scrolls the hovered pane, click selects+focuses,
  double-click edits the field.

# Stack and chrome

- charm v2 (`charm.land/bubbletea/v2`, `charm.land/bubbles/v2`).
- Full-screen (altscreen); below 60x12 the shell shows a too-small
  guard.
- Colors are adaptive (`LightDark(isDark)`); **every** visible concept
  registers a style in ADR-0003's palette registry — no inline styles in
  shell code (`TestPaletteRegistry_noInlineStyles`,
  `TestNoDeadStyles`).
- The footer: a spinner with the running operation's name or the message
  slot (success notes stand, failures persist until the next user
  action), then the focused pane's key hints rendered from the binding
  table.
- Modals render centered over the ANSI-stripped, dimmed base (lipgloss
  v2 layers on a canvas); the top modal owns all input; `esc` pops.
  Session events (apply) are compositor-level messages — they flow while
  any modal is open.
- Mouse: wheel scrolls the hovered pane, click selects and focuses,
  double-click edits a workspace field; the palette and modals take
  clicks too. Keyboard is primary; nothing is mouse-only.

# Apply UX

Apply runs **inside** the shell, over the service apply session
([service API](service-api.md)):

- **Entry**: `P` anywhere on the base. The apply area's read-only
  `Preview()` classifies the would-be session first; `p` shows the same
  classification as the read-only plan modal.
- **Gates, in order**: a destructive confirm when steps will overwrite
  existing files (copy-mode dotfile destinations on disk, named —
  backups are taken per issue 0025), then one elevation modal when the
  plan carries privileged steps (every reason listed; the password is a
  zeroed `[]byte` checked through `sudo -k -S -v`; three failures abort;
  `esc` aborts before anything is touched). Declining any gate writes
  nothing.
- While the run lives, the header's `▶ apply` badge and the footer carry
  status; `a` opens the apply detail inspector (step rows, output tail
  with `j`/`k`, verdict). Closing it never cancels; `ctrl+c` inside asks
  first. Cancellation kills the process group; the cancelled verdict
  names the interrupted step and the resume cursor names the last
  completed one (contract 2).
- When a step needs the terminal, the session's Handover seam delivers
  the child to the program (`tea.ExecProcess`); the child gets the real
  stdio — interactive hooks work, and with a fresh sudo timestamp sudo
  does not re-prompt. A concurrent apply elsewhere surfaces as the typed
  already-running refusal (contract 11).

# Fog

Previewing a *different* account's resolution, an undo system, and a
command-line-in-the-TUI stay out. Structural-section editing (secrets,
mounts, smb, nested when trees — systemd units have landed) continues
under issue [0074](../issues/0074-structural-section-editing.md).
