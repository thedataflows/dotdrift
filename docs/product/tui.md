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
  hints for the focused pane — the pane's primary verbs (enter, a, d,
  ctrl+s in the workspace, joined by ctrl+z while a draft holds staged
  changes) plus `/ ? q`, rendered from the binding
  table; scrolling yields the footer to the verbs, and `?` lists
  everything (0078).
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
  the nav. On the base it clears an applied nav filter first (0081). No
  per-view fallthrough.
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
  draft. A yanked module row carries `✓` (0093's mark set, below).
  Expansion state is remembered per session, and a reload keeps
  the selected row selected (clamping when the row is gone). Skipped or
  failed modules stay visible, greyed, naming the reason on the row;
  while the read is in flight the pane shows placeholder rows; an empty
  profile shows `(no modules)`. Rows truncate to the pane width.
- **The nav filter** (0081, T-tui-navfilter). `/` on the nav pane
  filters the module list in place — no modal. Typing narrows the rows
  live (the palette's own fuzzy matcher, declaration order kept; layer
  children follow their module), `j`/`k`/arrows move within the
  matches with the workspace following, and the query shows at the
  pane bottom — the `/ demo▏` cursor bar while typing, a named
  `/ demo · esc clears` after enter keeps the filter applied. `esc`
  removes the filter (while typing, after enter, or from the base esc
  rule). The cursor keeps its module across query changes while it
  still matches; an empty result names the query. Mouse clicks work
  on the filtered rows and commit the typing mode.
- **The workspace** (T-tui-workspace). `module.toml` rendered as a
  sectioned read surface over the 0065 config seam (strict decode + raw
  text; the workspace never parses TOML): fixed-order sections — meta,
  packages, links, writes, when, hooks, systemd.units, tools, secrets,
  mounts, smb — where a section shows what is set; an empty section is
  its header alone. The title line carries the layer tabs
  (`base · user cri · host myhost`, active bracketed) in sync with the
  nav both ways: selecting a layer child moves the tab, `L` cycles the
  tabs and moves the nav cursor. An overlay tab (anything not base)
  states the merge rule under the title — only packages, tools,
  dotfiles, hooks, mounts, smb merge; meta, scope, description, and
  when come from the base file. A module with a superuser overlay shows
  `needs root`. A broken `module.toml` no longer kills the shell: the
  interactive load (`profile.LoadTolerant`; strict `Load` stays the
  plan/apply path) greys the module's nav row with the error, and the
  workspace opens the file read-only — the error named relative to the
  layer dir, the raw text visible, every line width-clamped. `j`/`k`
  walk selectable rows (hint rows are skipped; an empty section's
  header and the structural families' headers are selectable rest
  points, and a header under the cursor renders the cursor bar) so
  T-tui-editing hangs edit mode off the same cursor, and the title line
  names the cursor's section (`demo [base] · writes`) so an empty
  stretch of surface still says where you are.
- **Inline editing** (T-tui-editing). `enter`/`e` turns the field under
  the cursor into an input without leaving the workspace; the tier-1
  error renders at the field live. The input's caret is a reverse-video
  block on the character under it — never a glyph inserted into the
  text, so the tail never shifts a cell (0092) — and bracketed paste
  lands at the caret with control runes stripped (0091); the same paste
  reaches every text input in the shell (dialog fields, the add form,
  the nav filter, the palette, the elevation password). Fields whose value set is closed
  open a choice picker instead (issue [0076](../issues/0076-tui-choice-editors-and-location.md)):
  `scope` (user/system), mount `state` (enabled/disabled), the boolean
  fields (`false`/`true`), smb `avahi` (`true`/`false`/`unset`), and
  the systemd `Type`/`Restart` directives — arrows walk, enter picks,
  clicking the selected row picks, and picking the effective value
  stages nothing, so a picker field can never hold an invalid value.
  Committed edits splice through the
  profile family encoders into the file-scoped draft (0065), which
  re-decodes on every commit — a draft can never hold unparseable text.
  Dirty shows per row, on the tab line, and on the nav rows. Drafts are
  file-scoped: navigation and layer switches never prompt. `ctrl+z` /
  `ctrl+shift+z` step the draft back and forward one committed change —
  field edits, adds, removes, and raw-line repairs are all one undo step
  each, the cycle caps at 50, and a new commit truncates the redo branch
  (0079). Rows announce their gesture, computed from the same registry
  that decides behavior so the mark cannot lie: `✎` an editable field,
  `◂▸` a closed set that cycles, `＋` an addable header or container; a
  committed edit's `●` replaces the mark (0079). Left/right on a
  closed-set row cycles its value in place — wrapping, one undo step per
  flip, no modal — while enter keeps the full picker (0079). `ctrl+s`
  saves through the untouched 0065 pipeline (staged tier-1 errors and
  the tier-2 cross-check block in place; a disk-hash conflict opens a
  reload-or-keep modal); `D` discards with a confirm naming module,
  layer, and change count; `a` opens a labeled add form for the section
  under the cursor (0077), `d` removes the row's thing with confirm
  (0087) — an entry, a group, a scalar, or one field of an entry; the
  uniform rule covers packages, tools, hooks, links/writes rows, systemd
  units and their directives, secrets/mounts/smb containers and fields,
  smb scalars, and when groups and leaves (meta rows, headers, and
  hints stay dead to `d`). Enter is the
  primary action in context (0078): on a row that owns no field — an
  empty section's header or a structural container — enter falls back
  to the same add form `a` opens, so the section under the cursor is
  reachable without knowing a second key. On a links row enter/e opens
  the link modal instead of the inline input (0095): a link is a
  target/source pair — the target is the dotfiles map key — so the
  prefilled form (the add form's twin) edits both, and committing a
  changed target renames the entry, mode preserved. Fields are typed
  inputs (0094): a writes block and a hook command open a **multi-line
  editor** — enter splits, backspace joins, paste keeps its newlines
  (unlike the single-line sanitizer), `ctrl+enter` commits, `esc`
  cancels, and `ctrl+g` toggles source coloring guessed from the
  target's extension (the caret's own line stays plain so the
  reverse-video caret survives) — and path fields (smb share `path`,
  mounts `source`/`destination`) open a **file picker** instead of the
  inline input: a desktop-style listing (dirs first, symlinks resolved,
  dotfiles behind `.`) with arrows/pgup/pgdn/home/end, `/` filtering
  the current listing, `enter` picking per mode, `ctrl+enter` picking
  the shown directory, and a `ctrl+l` location bar for typed or pasted
  paths — tilde expands, and a typed path is accepted exactly as typed:
  it need not exist, not even its parents (0099; link targets to
  create, server-side share paths) — existence rules belong to the
  field's commit validation, which renders its refusal in the modal.
  The picker opens on the field's
  current value (0096): a valid path shows its parent directory with
  its own entry selected — enter re-confirms it, the siblings are one
  keystroke away — a missing path climbs to the nearest existing
  ancestor, and a dotfile path reveals hidden entries so it can be
  selected. The listing's first row is `../` whenever the shown
  directory has a parent (0097): enter and right on it navigate up
  like left (it never picks, not even in dirs mode), home and
  wheel-up reach it, and the default cursor skips it, landing on the
  first real entry. Form fields that take paths — onboard
  `paths`, the link forms' `target` and `source` (a target may be a
  directory symlink, so it browses in either mode too), the writes add form's `target`,
  the mounts/smb field forms' path values — browse with `ctrl+o`
  instead, because enter already means *commit the form*; the onboard
  pick appends to its space-separated list. A link `source` browse is
  module-relative (0100): the picker roots at the active layer's module
  directory — an empty field opens inside it, the edit modal's relative
  value seeds there with its entry selected — and the pick stores the
  module-relative path the resolver expects; a pick outside the module
  directory refuses inside the picker (`resolveSource` would reject it
  at resolve time anyway), while a relative location-bar path stores as
  typed — it may exist in another layer. Every pick and every editor
  commit rides the same validation, splice, ledger, and undo pipeline a
  typed commit runs. A broken file
  edits as raw text lines
  (`e` on a line); a repair that parses unlocks the structured surface,
  and its save sends the whole repaired file as the raw candidate
  (`SaveRequest.Raw` — the splice step swaps out, every other pipeline
  check stands). The structural families all edit in place now (below);
  structural-section editing landed from issue
  [0074](../issues/0074-structural-section-editing.md).
- **Structural editing** (0074 grammar, 0075 disclosure). The
  structural families join the inline grammar as container + field rows,
  and a row renders only when it differs from the zero value. A systemd
  unit is a container row (d removes with confirm) whose SET directives
  render beneath it as indented rows; directive values are typed TOML —
  `ExecStart = /usr/bin/demo` needs no quoting, while `"x"`, `30`,
  `[a, b]`, and inline tables land with their TOML types. Secrets,
  mounts, and smb shares edit the same way — one container per entry,
  its set fields beneath it. The add forms reach everything the
  disclosure hides: `a` on a section header adds the section's entry
  (an empty section's header is selectable; the structural families'
  headers always are — they are the entry-level scope), `a` on a
  container adds a field INTO the entry (the field is a closed choice),
  `a` on the smb header offers share/group/users/avahi beside bare
  share names, and `a` on when offers leaf or `and`/`or`/`not` (nesting
  deeper) at any depth. Editing a field to its zero value clears it and
  the row
  disappears (required fields refuse to empty); `d` on the field row
  unsets it directly, and removing a required field leaves an invalid
  entry the save names (0087). A container with
  nothing set yet shows a dim `· a adds "field = value"` hint. Entries
  resolve could never accept are refused at save (missing mount
  source/destination/type, empty share path, an empty when group —
  nothing to evaluate) — tier-2, in place. meta keeps its description
  and scope rows: identity, not options.
- **Add forms** (issue [0077](../issues/0077-tui-add-forms.md)). `a`
  opens a centered form titled with the entry and its destination
  (`add package · demo`) — the inline grammar input it replaced
  rendered nowhere, so users typed blind. Rows are labeled, identity
  first (name, target, unit, directive), closed sets as `< >` choices
  (packages present/absent, hooks pre/post, smb what, when kind, the
  structural field choices), values free text; letters are never
  navigation, so `jdk` types as itself. Enter synthesizes the
  pipeline's grammar and commits through the same applyEdit path —
  validation, splice, ledger, and the cursor landing on the new row —
  and a refusal keeps the form open with the error inside it. writes is
  addable now: a line kind (target + line text, spaces and `=` allowed)
  or a link kind (target + source). The line lands in writes, the link
  in links.
- **The modal family** (T-tui-modals). One confirm component — question
  title, consequence body with full target identity, `y` confirms and
  every other key (or `esc`) cancels — backs dirty-quit (`q` with
  drafts), row removal, module deletion, and the destructive-apply gate.
  `m` on a module opens the 0072 manage menu as a modal (create / move /
  delete with the orphan preview), domain logic untouched; any
  successful write that changes the profile — manage ops, onboard,
  generate (0085) — re-runs the startup profile read so the nav tree
  shows the change without a restart (restore writes live targets only,
  so it does not reload). `O` on the nav overrides the
  selected module in one step (0082): it creates
  `users/<you>/modules/<dir>/` seeded with a comment-only `module.toml`
  — an empty overlay overrides nothing, and delete module undoes it, so
  there is no dialog and no confirm — then the reload lands the
  workspace on the new file, tab and cursor on it. `P` runs
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
- **The palette** (T-tui-palette). `/` on the workspace pane opens the
  fuzzy palette — the teleport path (the nav pane's `/` is the 0081
  in-place module filter): modules with overlay layers as separate entries
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
  `P` apply, `d` remove row / `D` discard draft); esc pops the top layer
  (modal → edit → filter → focus to nav, 0081); `ctrl+z`
  steps the active draft back one committed change and `ctrl+shift+z`
  redoes (0079) — undoing past the first change returns the clean file,
  and confirms remain the safety net for what undo cannot reach (saves,
  quits). `/` is pane-scoped: the nav pane's filters the module list in
  place, the workspace pane's opens the palette (0081) — the footer
  hint resolves the verb from the table per focused pane.
  `p` opens the read-only
  plan modal (step classification, sudo reasons, overwrite counts —
  never credentials); `w` opens the writes menu (onboard / restore /
  generate as absorbed dialogs); `n` opens module creation; `o` opens
  the onboard dialog prefilled with the selected module and layer
  (either pane, 0079); `O` overrides the selected module into your user
  layer in one step (nav pane, 0082). Every form dialog gates its run
  the same way (0089): enter arms a confirm whose prompt — and a `y
  runs · n/esc back` footer — renders only while the gate is armed, and
  `n` disarms back to the form (delete module's `n` steps back to the
  manage menu). `space`/`y` on the nav pane toggle the selected module's
  yank mark (0093): a non-empty yank set scopes `p` and `P` to exactly
  those modules — the set rides the CLI's module filter
  (`ApplyOpts.Modules`), snapshots at the `P` press and rides the gates
  into the run, the plan modal names the scope under its title, and a
  nav reload prunes ids whose module disappeared — while an empty set
  keeps the default: all modules.
  `pgup`/`pgdown` move the cursor a visible page and `home`/`end` jump
  to the first/last row, in both panes; the workspace's visible window
  always follows the cursor (0075). Mouse parity
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
- **Values read as content, labels recede** (0080). The `value` hue is a
  bright neutral (255 on dark, 234 on light — the brightest chrome on
  screen, never an accent hue): it colors `rowText` — dialog/form
  values, picker rows, workspace and nav rows — while fixed labels use
  `fieldLabel` (muted) and placeholder hints stay dim. Filled vs unfilled
  reads by color, backed by the hint's parentheses; chrome and run
  output stay default.
- The cursor row is the bubbles list-delegate treatment: a left bar in
  the highlight hue plus bold accent text (the `cursorRow` registry
  entry), plain rows keeping a two-space lead so text columns align.
  Every cursor uses it — nav rows, workspace rows, the field input, the
  palette's query and selected row, modal menu and form rows (0075).
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
command-line-in-the-TUI stay out. Structural-section editing is complete
per issue [0074](../issues/0074-structural-section-editing.md).
