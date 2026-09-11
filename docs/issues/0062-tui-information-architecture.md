---
type: Issue
title: TUI information architecture
description: Decide the tree model, pane behaviors, keybindings, and chrome of the two-pane tui, grounded in what the bubbles inventory says is buildable.
tags: [wayfinder, grilling, tui]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0062: TUI information architecture

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [ADR-0003](../adr/0003-tui-shared-palette.md)
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
