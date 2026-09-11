---
type: Issue
title: TUI information architecture
description: Decide the tree model, pane behaviors, keybindings, and chrome of the two-pane tui, grounded in what the bubbles inventory says is buildable.
tags: [wayfinder, grilling, tui]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0062: TUI information architecture

- **Type**: task
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [ADR-0003](../adr/0003-tui-shared-palette.md), [research 0058](../research/0058-bubbles-layout-inventory.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Blocked by**: [0058 Bubbles &amp; layout inventory](0058-bubbles-layout-inventory.md)
- **Closing commits**: none

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
