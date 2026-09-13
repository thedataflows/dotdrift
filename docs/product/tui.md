---
type: Specification
title: TUI
description: dotdrift tui — the one interactive surface; two-pane shell, full-schema editors, streamed apply.
tags: [product, tui]
timestamp: 2026-09-12T00:00:00Z
---

# Command surface

`dotdrift tui [--profile PATH]` (default `.`) opens the two-pane shell at
a profile root. The TUI is dotdrift's **one interactive home**
([ADR-0007](../adr/0007-generate-cli-only.md)) — every other command is
strict flag mode, and everything interactive lives here. The plan view's
apply gate is the TUI's **only** write path into convergence; profile
*files* are written only by the editors and the module-management dialogs,
both below.

# The compositor (M15, landing task by task)

M15 ([milestone](../milestones/m15-tui-compositor.md), issue
[0073](../issues/0073-tui-compositor-redesign.md)) rebuilds the
presentation layer as one shell plus a modal stack. It replaces the view
stack described below as its tasks land; the old shell stays the default
until T-tui-cleanup deletes it. The new shell is reachable now behind the
hidden flag `dotdrift tui --compositor`.

Shipped so far (T-tui-compositor):

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

# Stack and chrome

- charm v2 (`charm.land/bubbletea/v2`, `charm.land/bubbles/v2`) — chosen
  for the official tree bubble (issue 0062-D1; research 0058).
- Colors are adaptive (`LightDark(isDark)`); **every** visible concept
  registers a style in ADR-0003's palette registry — no inline styles in
  shell code.
- Help is bubbles `help.Model`: the status bar shows the focused pane's
  key hints (ShortHelp), `?` toggles the full view.
- Header, one line: profile root, current host/user context (read-only in
  M14), dirty indicator when any editor draft is unsaved.

# Layout

Two panes, exactly one focused at a time; focus is the border color
(indigo focused, dim unfocused — the prototype-approved treatment,
issue 0063).

- **Tree, left**: fixed 30% width, clamped to 22–44 columns.
- **Main pane, right**: one active view at a time.

Views live on a **view stack**: selection opens a view, `esc` pops.
Dirty editor state survives navigation per module for the session. Apply
is a mode replacing the main pane, not a stacked view.

# Tree

Three root groups (0062-D2/D8):

- **Modules** — one node per module merged across layers (contract 1)
  with an overlay-count badge; an expandable overlay-stack node under the
  module lists its origins as children (`base/`, `hosts/<h>/`,
  `users/<u>/`, the superuser overlay), so "root touches this module" is
  visible where the module lives. Sections render in the module's main
  pane view, never as tree leaves.
- **Accounts** — hosts, users, and the superuser overlay (per its
  visibility rule, issue 0029) as plain nodes carrying account facts. No
  per-account drift views; the only other-account affordance is
  ADR-0006's read-only notice, shown in the status view.
- **Profile** — **Plan** and **Status** are views; **Onboard**,
  **Restore**, and **Generate** are profile-scoped actions opening dialog
  views with confirms. Module- or layer-scoped generate hangs off
  module/layer-node context menus instead.

# Selection and reading

Resolved by default: a module shows the resolved overlay stack for the
current host/user — what `plan` sees (the CLI has no read-path override,
and neither does the TUI). Each layer's raw declaration is on demand via
the toggle or the overlay node. Previewing a *different* account's
resolution is out of M14 scope (declared fog).

# Keys

Vim-ish baseline: `j`/`k` and arrows move, `tab`/`shift-tab` switch
panes, `enter` opens, `esc` pops/back, `q` quits with dirty-confirm, `?`
toggles help, `g`/`G` jump. Global keys are reserved by the shell. Tree
filtering (`/`) is deferred. Navigation is keys + context menus only —
no command palette and no `:` command line (deferred with review,
0062-D6/0065-D4; fog until a user story asks for a second navigation
model).

# Apply UX

Apply runs **inside** the TUI, streamed, over the service apply session
([service API](service-api.md)):

- **Entry**: `a` on the plan view opens apply — the mode replaces the
  main pane (0062-D4); the view stack stays underneath, untouched.
- The plan view carries per-step diffs and the **gate**: before running,
  the apply area's read-only `Preview()` classifies the steps and
  announces "N steps will take the terminal" with reasons. Confirming
  the gate starts the session — this is the TUI's only door to apply;
  declining writes nothing.
- Events pump into bubbletea (`p.Send` off one drain goroutine); the
  progress list and output pane are TUI-internal view-models, including
  output coalescing (ring buffer + tick — raw line events are never
  rendered one-per-frame).
- When a step needs the terminal, the TUI hands it over with
  `tea.ExecProcess` (alt-screen suspend/restore is bubbletea's job); the
  child gets the real stdio — sudo prompts and interactive hooks work.
  While a handover child owns the terminal the TUI is suspended
  (bubbletea runs the exec on the event loop), so nothing in the shell
  can fire until the child exits.
- Cancel is the explicit gated exit (`x`, or `q`/`ctrl+c` while running;
  `esc` never pops a running apply). It kills the process group
  immediately; the cancelled-state screen names the interrupted step and
  the resume cursor still names the last completed step (contract 2) —
  a kill the session itself performed classifies as Cancelled, not
  Failed. A concurrent apply elsewhere surfaces as the typed
  already-running refusal (contract 11).

# Editors

First-class structured editors for every `module.toml` section — one
schema-driven frame with section adapters, not fourteen bespoke tools
(0065):

- **Entry**: `e` on a raw origin view opens the editor — the tree
  position *is* the layer picker, and the editor chrome names the exact
  file. Opening "the winning layer" or choosing a layer at save are both
  rejected designs.
- **Keys**: the editor vocabulary rides under the shell's reserved
  globals — `[`/`]` switch section, `enter` edits the focused field,
  `n`/`x` add/delete a row, `space` toggles, `v` switches a dotfiles
  entry's variant (or a hook row's spelling), `<`/`>` reorder rows, `s`
  saves, `r` reloads after a disk conflict, `esc` steps back and guards
  through the dirty-confirm (save / discard / cancel).
- **Draft**: the unit of editing is one (module, layer) file — raw text +
  baseline disk hash + typed config copy. Multiple dirty sections of one
  file save in one splice. Dirty means deep-compare against the baseline;
  `esc`/`q` dirty-confirm is the only guard (no undo in M14).
- **Save pipeline** (in order): field validation → encode the edited
  sections → splice into the file's raw text → strict-decode the spliced
  text (the round-trip proof that contract 19 holds by construction) →
  the resolve-level checks (source containment, source existence,
  cross-module conflicts, scope) → disk-hash check (an external change
  refuses the save and offers reload — no merge) → atomic tmp+rename.
- **Splice semantics**: untouched sections keep their bytes, comments,
  and position; edited sections re-encode canonically and
  deterministically — interior comments of an edited section re-encode
  away (the named cost, already paid under onboard). A broken
  `module.toml` opens read-only with its schema error.
- **Sections**: form adapters for module keys (`id`/`app`/`description`/
  `disabled`/`scope`), packages (ordered present/absent), tools, secrets
  (short form when only `env`), mounts and smb — the latter two prefill
  from `internal/generate`'s assembly machines, so **the input an editor
  prefills equals the input `generate` assembles** (contract 15). Custom
  view models only where the schema is not form-shaped: **dotfiles**
  (entry list, whole-file vs edit variants, contract-18 field
  exclusivity), **when** (validated text area with positioned errors),
  **hooks** (ordered rows, one layer's array), **systemd.units**
  (directive passthrough with structural checks).
- **Module management** (`m` on a module or origin selection opens the
  context menu; the tree position is its context): create = minimal
  scaffold (app + layer choice — base, this host, this user) with
  sections added through the editors; move = wholesale module-dir
  move, refused when the target layer already has the module; delete =
  confirm dialog pre-computing which referenced sources become orphans.
  The menu's entries open in place as forms; `esc` steps back through
  the menu, then out; a successful op reloads the modules read so the
  tree reflects the change. Content is never merged — contract 7 stays
  file-literal.
- **OS accounts**: the Accounts group manages `users/<u>/` overlay
  lifecycle and shows ADR-0006's notice. `bootstrap.users` is *emitted*
  mise config, not profile schema; `smb.users` is an ordinary `[smb]`
  field annotated "becomes OS accounts at apply".

# Non-goals for M14

Command palette / `:` line; another-host resolution preview; editor
undo/redo; structured when-tree builder; theming mechanism beyond the
ADR-0003 registry; large-profile performance work. All are declared fog
on the design map (issue 0057), not silently dropped.

# References

Design map [0057](../issues/0057-dotdrift-tui-design-map.md); research
[0058](../research/0058-bubbles-layout-inventory.md)/[0059](../research/0059-apply-streaming-tty-precedents.md);
tickets [0062](../issues/0062-tui-information-architecture.md) (IA),
[0063](../issues/0063-two-pane-shell-prototype.md) (approved shell
prototype), [0064](../issues/0064-apply-session-tty-suspend-design.md)
(apply session), [0065](../issues/0065-full-schema-editor-suite-design.md)
(editors), [0066](../issues/0066-wizard-absorption-contract-15.md)
(wizard absorption); [service API](service-api.md); ADR-0003 (palette),
[ADR-0007](../adr/0007-generate-cli-only.md) (one interactive home),
ADR-0008 (service doorway); milestone M14; contract invariants 1, 2, 7,
11, 15, 18, 19.
