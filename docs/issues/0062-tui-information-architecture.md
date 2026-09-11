---
type: Issue
title: TUI information architecture
description: Decide the tree model, pane behaviors, keybindings, and chrome of the two-pane tui, grounded in what the bubbles inventory says is buildable.
tags: [wayfinder, grilling, tui]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0062: TUI information architecture

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [ADR-0003](../adr/0003-tui-shared-palette.md), [research 0058](../research/0058-bubbles-layout-inventory.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Blocked by**: [0058 Bubbles &amp; layout inventory](0058-bubbles-layout-inventory.md)
- **Closing commits**: pending

## Question

Left tree pane, right main pane — but what exactly is in them, and how
does one drive the whole app from the keyboard?

- Tree model: profile → layers (`modules/`, `hosts/<h>/`, `users/<u>/`)
  → modules → sections (`dotfiles`, `packages`, `tools`, `mounts`,
  `smb`, `systemd.units`, `hooks`, `secrets`, `bootstrap.*`)? Where do
  profile-level views (plan, status, drift) hang? How do host/user
  overlay relationships render (a module that exists in base and user
  layers is one module with overlays — contract 1)?
- Selection semantics: does selecting a module in the tree resolve its
  overlay stack for the current host/user (like `plan` does), or show
  each layer's raw declaration? Both, toggleable?
- Main-pane view types: read views vs editors vs apply/diff surfaces;
  how the pane switches and what state persists.
- Keybindings and focus model: j/k tree navigation, tab pane switch,
  enter open/edit, esc back — the vim-ish baseline vs arrows-only;
  help pane (`?`).
- Chrome: header (profile root, current host/user context), status bar,
  how ADR-0003's palette registry extends to the new surfaces.
- Command surface inside the TUI: is there a command palette / `:`
  command line (apply, onboard, generate invoked from where?), or is
  everything context-menu/keypress driven?
- Charm stack line: the pinned v1 (maintenance-mode) vs GA v2
  (`charm.land/*`) — research 0058 shows the official tree bubble exists
  only on v2 while v1 would hand-roll selection over lipgloss's
  render-only tree, so this choice bounds every bullet above; decide it
  here, and the prototype ([0063](0063-two-pane-shell-prototype.md))
  pins it in `go.mod`.

## Decisions

Round 1 — accepted 2026-09-11:

- **D1 Charm stack**: v2 (`charm.land/bubbletea/v2`,
  `charm.land/bubbles/v2`) — GA with the official tree bubble (research
  0058); the huh v1 wizard forms keep working on the old module paths
  until [0066](0066-wizard-absorption-contract-15.md) absorbs them. The
  prototype ([0063](0063-two-pane-shell-prototype.md)) pins it in
  `go.mod`.
- **D2 Tree model**: module-centric — three root groups (**Modules**,
  **Accounts**, **Profile**). One node per module merged across layers
  (contract 1) with an overlay-count badge and an expandable
  overlay-stack node; sections render in the module's main-pane view,
  not as tree leaves. Accounts lists hosts, users, and the superuser
  overlay (glossary terms); Profile holds plan, drift/status, onboard,
  restore, generate entry points.
- **D3 Selection semantics**: resolved by default — the resolved overlay
  stack for the current host/user (what `plan` sees; the CLI has no
  read-path override), with each layer's raw declaration + origin
  markers on demand via the toggle / overlay node. Previewing *another*
  host's resolution stays out of M14 scope (fog).
- **D4 View model**: view stack, one active view — selection opens a
  view (read/edit/dialog/apply), `esc` pops; dirty editor state
  survives navigation per module for the session; apply is a mode
  replacing the main pane (cancel gated per
  [0064](0064-apply-session-tty-suspend-design.md)).
- **D5 Keybindings**: vim-ish baseline (`j/k` + arrows, `tab`/
  `shift-tab` pane switch, `enter`, `esc`, `q` with dirty-confirm, `?`
  toggling ShortHelp/FullHelp via bubbles `help.Model`, `g`/`G`); exactly
  one focused pane; global keys reserved by the shell; tree filtering
  (`/`) deferred.
- **D6 Command surface**: keys + context menus only for M14 — apply
  hangs off Profile→plan's gate, onboard off Accounts, generate off a
  module/layer node; no `:` command line, no palette (fog until
  [0065](0065-full-schema-editor-suite-design.md)'s editor suite exists
  to command).
- **D7 Chrome & palette**: header = profile root, current host/user
  context (read-only in M14), dirty indicator; status bar = focused
  pane's key hints (`help.Model`) + apply state when running. Palette
  rule: every new visible concept registers a style in ADR-0003's
  registry — no inline `lipgloss.NewStyle()` in shell code. The
  registry→bubbles mechanism stays the map's fog item.

Round 2 — accepted 2026-09-11:

- **D8 Accounts & superuser overlay**: accounts as nodes, origins as
  markers — the Accounts group lists hosts, users, and the superuser
  overlay (per its visibility rule, issue
  [0029](0029-superuser-user-overlay-visibility.md)) as plain nodes carrying
  account facts; a module's expandable overlay-stack node carries the
  origin markers (`base/`, `hosts/<h>/`, `users/<u>/`, superuser), so
  "root touches this module" is visible where the module lives. No
  per-account drift views; the only other-account affordance is
  ADR-0006's one notice (read-only, in the status view). Mirroring
  modules under account nodes would violate contract 1's one-module
  rule and was rejected.
- **D9 Status/drift view**: full status read-model parity — the
  complete `status` report (symlink validity, orphans, tool status, the
  ADR-0006 account notice) in a scrollable read view, rendered with
  [0061](0061-service-layer-architecture-cli-migration.md)-D5's
  canonical renderer, so CLI/TUI fact-identity is by construction. This
  resolves the map's "status/drift view scope" fog item.
- **D10 Profile group contents**: **Plan** and **Status** are views;
  **Onboard**, **Restore**, **Generate** are profile-scoped actions
  opening D4's dialog views with confirms; module/layer-scoped generate
  hangs off module/layer-node context menus. The plan view carries
  per-step diffs, and its gate is the TUI's **only** write path into
  convergence — consistent with D6 and the session design in
  [0064](0064-apply-session-tty-suspend-design.md).
- **D11 Fog bookkeeping & closure**: close 0062 after this round. Map
  fog: *remove* "status/drift view scope" (D9) and "multi-account
  presentation" (D8); *add* "another-host resolution preview" (D3's
  deferral) and "command palette / `:` line" (D6's deferral); the
  theming-mechanism and large-profile-performance items stand.

## Resolution

**The two-pane TUI is a module-centric tree over a view stack, on
charm v2, with exactly one door to apply.** Left pane: Modules /
Accounts / Profile groups (D2) — one node per module merged across
layers (contract 1), accounts as plain nodes with overlay origins
marked on the module (D8). Right pane: one active view at a time —
resolved-by-default module stacks (D3), full-status parity (D9), plan
with per-step diffs, dialogs, streamed apply (D4) — with dirty editor
state surviving navigation. Vim-ish keys, one focused pane, help from
`help.Model` (D5); keys + context menus only, no palette (D6); minimal
chrome, every new concept registers in ADR-0003's registry (D7).

Consequences carried consciously: v2's younger ecosystem and the huh
v1 coexistence are the accepted cost of the official tree bubble (D1);
another-host resolution preview and a command palette are declared fog,
not silently dropped (D3/D6/D11); the apply gate being the only write
path keeps convergence single-threaded through the session design in
[0064](0064-apply-session-tty-suspend-design.md).

Unblocks [0063 two-pane shell prototype](0063-two-pane-shell-prototype.md)
and [0065 editor suite](0065-full-schema-editor-suite-design.md) (each
was blocked only by this ticket) and leaves
[0064 apply session](0064-apply-session-tty-suspend-design.md) on the
frontier with them. 0066 waits on 0065; [0067](0067-assemble-tui-design-set.md)
waits on 0063/0064/0065/0066.
