---
type: Specification
title: TUI views — status, plan, profile, apply options, gh-dash polish
description: Design for transposing the dotdrift CLI read side and apply options into the TUI shell, with the gh-dash interaction polish, in five slices.
tags: [product, tui, design]
timestamp: 2026-10-18T00:00:00Z
---

# TUI views design

## Intent

The CLI has three kinds of work: read (`detect`, `modules`, `plan`,
`status`), edit declarations, and converge (`apply`, `restore`). The TUI
covers edit and converge. The read side is missing: a user inside the
TUI cannot see what is out of sync on the live machine, cannot see the
merged plan, and cannot see the detected facts. They must quit and run
CLI commands. This design closes that gap and adds the apply options the
CLI has and the TUI lacks (`--force`, `--backup`, `--diff`, section
selection).

The service layer already has every read: `ReadsArea.Modules / Plan /
Status / Diff / Detect`. The TUI calls only `Modules`. This is wiring
and screens, not new domain logic.

Layout direction (approved): the gh-dash shape — top-level views, each
with its own list and detail. Not the current two-pane editor stretched
over four jobs, and not a dashboard home screen.

## Goals

1. A **Status view**: the drift list of `dotdrift status`, grouped by
   section, with the resume line. Enter on a finding jumps to that
   module in the Modules view. `P` on a finding applies with that
   module scoped (the existing yank mechanism). `r` re-probes.
2. A **Plan view**: the full merged plan of `dotdrift plan` (packages,
   tools, dotfiles, hooks, systemd, mounts, smb), per section, plus the
   copy-mode diff preview the CLI shows under `--diff`.
3. A **Profile view**: the `detect` facts (host, user, os, kernel,
   distro, GPU, backend), the profile paths, and the resume state. Read
   only.
4. **Apply gate options**: in the existing apply gate, toggle force,
   toggle backup, and toggle sections, with the same meaning as
   `apply --force / --backup / --[no-]<section>`. Defaults match a plain
   `P` today (no force, no backup, all sections).
5. **Generate mounts volume list**: the CLI `--list-volumes` table
   inside the generate mounts dialog, so a source is picked, not typed
   blind (`WritesArea.Volumes` exists and is unused).
6. **Polish, from gh-dash**: a refresh key for the current view (`r`)
   and all views (`R`); re-read after any write or apply finishes;
   per-view loading and empty states; the footer task status shows a
   short ✓/✗ after an operation ends; section counts on view tabs.

## Non-goals

- `init` (create or clone a profile) and switching profiles inside the
  TUI. The shell opens one profile, as today.
- Elevated restore (`sudo install` targets). The restore dialog keeps
  refusing them and naming the CLI command.
- User-defined keybindings and user shell-command keys (a gh-dash
  feature). No CLI counterpart; shell injection by design. Out.
- A dotdrift YAML config file. dotdrift config is `dotdrift.toml`; the
  theme keeps coming from the profile `[colors]` palette.
- gh-dash's shared mutable `ProgramContext` and focus-priority ladder.
  The compositor's modal stack and message flow stay.
- Pagination, network fetch, markdown rendering. All data is local.
- Previewing a different account's resolution (existing Fog item).

## Anti-goals

- A screen that shows stale drift as current. Every view names its
  state: loading, current, or failed. A failed probe re-run keeps the
  old list only with a visible "stale" mark, never silently.
- A second keymap source. The one binding table stays the only source;
  footer hints and `?` help keep rendering from it.
- New domain logic in the TUI. Every read goes through the service
  layer's narrow consumer interfaces (ADR-0008). The `Reads` consumer
  interface widens; the TUI does not call domain packages to read.

## The shell change

Today: header, nav (modules) left, workspace (one module layer file)
right, footer. One base screen plus a modal stack.

New: a **view row** under the header: `Modules · Status · Plan ·
Profile`, the active view marked. One key cycles views; number keys
jump. The binding table decides the exact keys (builder's call, one
line each, no conflict with pane keys: `tab` is pane focus and stays).

- **Modules view** = today's base screen, unchanged.
- **Status view** = left: findings list grouped by section with counts;
  right: detail of the selected finding (module, item, state, detail
  text; for a copy-mode dotfile, the live diff). Section filter keys
  narrow the list (builder's call: tabs or a filter key, one mechanism).
- **Plan view** = left: plan sections with counts; right: the selected
  section's merged entries. A diff toggle shows the copy-mode diffs
  (the `ReadsArea.Diff` data) in place of the entries.
- **Profile view** = one read-only list: facts, profile root, state
  path, resume line, other-accounts note (the `StatusRead.Others`
  data). No detail pane work beyond the list.

Each new view is a model of the same standing as `navModel` /
`workspaceModel`: it owns its cursor and scroll, renders into the right
or both panes, and receives keys only while its view is active. The
modal stack, the one `esc` rule, and the message slot do not change.
While a modal is open, it still owns all input.

View state lives for the session. Leaving a view and coming back keeps
its cursor. A write (save, manage, onboard, generate, restore) or a
finished apply marks the read views stale; `r` on a view re-reads that
view, `R` re-reads all views. No timer refresh: nothing changes unless
the user or an apply changed it, and apply already reports back.
(Builder's call: a focus-regain re-read is allowed if it is cheap and
cannot fire mid-edit; default off.)

## Data flow

- Status view reads `ReadsArea.Status` (facts, findings, others, state)
  on first entry and on `r`. Probing can be slow (package queries,
  `mise current`, sudo probes): it runs as a `tea.Cmd`, the view shows
  its loading state, and a second `r` during a probe is a no-op (or
  cancels — builder's call, name it in the plan).
- Plan view reads `ReadsArea.Plan` and, for the diff toggle,
  `ReadsArea.Diff`. Yank scope applies to both views as it does to
  `p`/`P` today: a non-empty yank set scopes the read; the view title
  names the scope.
- Profile view reads the facts the shell already loads, plus
  `ReadsArea.Status`'s state/paths data (shared with the Status view's
  read when both are loaded — one read, two views; builder's call how
  to share, a cached `StatusRead` on the compositor is fine).
- Apply gate options ride the existing flow: `ApplyOpts.Force`,
  `ApplyOpts.Backup`, `ApplyOpts.Sections` already exist on the service
  type. The gate renders three toggle rows above the existing
  destructive/elevation gates; the snapshot rides the `P` press like
  the yank snapshot does (0093). `BackupTaken` events already render.

## Keys (new, all in the one table)

- Cycle views / jump to view (both panes).
- `r` refresh current view, `R` refresh all (both panes, not while a
  text input or modal owns keys).
- In Status: `enter` jump to the finding's module (Modules view, layer
  of the winning entry when known, else base), `P` apply scoped to the
  finding's module.
- In Plan: a diff toggle key.
- Every existing key keeps its meaning inside the Modules view.

## Slices (build and ship in this order)

1. **Shell views + Profile view.** The view row, view cycling, the
   stale/refresh mechanism, Profile view (cheapest read, proves the
   frame). Issue + tests + goldens.
2. **Status view.** Findings list, section filter, detail with copy
   diff, enter-to-module, scoped `P`, `r`. The core slice.
3. **Plan view.** Merged plan sections + diff toggle.
4. **Apply gate options.** Force/backup/section toggles in the gate.
5. **Generate volumes + polish.** `--list-volumes` in the mounts
   dialog; per-view loading/empty states finished; footer ✓/✗ end
   marks; section counts on the view row. (Any of these that slices
   1–4 already need land there; slice 5 is the remainder.)

Each slice: one issue in `docs/issues/` (project rule), TDD red first,
`go test ./...` and `go vet` green, goldens for new frames at 100x30
and 64x24, `docs/product/tui.md` and `docs/log.md` updated in the same
commit. Contract and ADR change only if a slice breaks an invariant —
none is expected; ADR-0008's doorway rule is the constraint, not a
casualty.

## Risks

- **Probe cost and hangs.** Status probes run package managers and
  `mise`; a hung probe must not hang the shell. The read is a command;
  the shell keeps rendering. Cancellation on quit already exists at the
  program level. A probe timeout is out of scope unless a slice proves
  one is needed.
- **Sudo probes.** Status probes elevate per probe for system files
  (contract 17). In the TUI the elevation modal validates sudo before
  the read (settled above), so no prompt reaches the altscreen; the
  read says "probing" while it runs. Slice 2 builds that gate first,
  before list work.
- **Golden churn.** New frames change goldens. Contained per slice.
- **Width.** The view row costs one header line at 64x24. The too-small
  guard (60x12) may need a raise; slice 1 checks.

## Left to the builder

- Exact view-cycle keys and the Status section-filter mechanism.
- Whether `r` during a probe is a no-op or a cancel.
- Internal model file layout (one file per view is the house pattern).
- The stale mark's exact render (a word in the view title is enough).

## Settled at builder-check (my calls, 2026-10-18)

- **Profile view does not probe.** It uses a light read: facts the
  shell already has, the plan read (no probes), the state file, and the
  other-accounts list. No full `ReadsArea.Status` run, no sudo. If a
  small service read is needed to carry state path + resume + others
  without probing, slice 1 adds it to the reads area — that is wiring,
  not domain logic.
- **Probe assembly lives in the service layer.** `ReadsDeps` gains the
  probe-builder dep (`ProbesFor`), wired in `cmd/tui.go` exactly as
  `cmd/status.go` wires the CLI today; the TUI never imports drift
  probe constructors (ADR-0008 holds).
- **Loads stay tolerant.** Status and Plan inherit the shell's tolerant
  profile load. A broken module never fails a view; the views show what
  resolved, and the Modules view already greys the broken row with its
  error. Strict load stays the apply path.
- **Multi-owner findings.** `enter` jumps to the first owning module;
  scoped `P` applies to all owning modules (the comma-joined owner
  list, split). Orphan findings jump to the module of that layer
  directory.
- **Rows and counts, CLI parity.** The Status list shows only drift
  and unknown findings, as the CLI report does. An all-OK section shows
  one "ok: all N checks passed" row. Counts are drift + unknown.
- **Gate toggles sit before Preview.** `P` opens the options gate
  (force / backup / sections) first; on confirm, one Preview runs with
  the snapshotted options, then the existing destructive and elevation
  gates follow from that classification. Choices reset on every `P` —
  one run, one set of options, like one CLI invocation.
- **Yank changes mark reads stale.** Changing the yank set in the
  Modules view marks Status and Plan stale (their titles name the
  scope they were read under). Scoped `P` on a finding applies to the
  finding's modules even when a yank set is active — the gesture is
  explicit and wins.
- **Volume pick fills `source` only** (`UUID=<uuid>`), snapshotted
  from the dialog's current layer/module/host/user fields. Type and
  destination stay the user's. Managed volumes render marked, as the
  CLI marks them, and stay pickable.

- **Sudo probes: elevation modal first (user's answer).** When a
  Status read will probe root-owned targets, the shell validates sudo
  once through the existing elevation modal (`sudo -k -S -v`, zeroed
  password, three failures abort) before the read starts; the probes
  then run elevated silently under the fresh timestamp, as the CLI's
  per-probe sudo does. No prompt ever reaches the altscreen. If
  elevation is declined or fails, the read runs unelevated and
  root-owned findings land as `unknown` with the reason — the probes'
  existing behavior when elevation is unavailable.
