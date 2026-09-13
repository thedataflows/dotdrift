---
type: Issue
title: TUI compositor redesign
description: Rebuild the TUI presentation layer as a base shell plus modal compositor — sectioned workspace, inline editing, elevation modal, fuzzy palette — replacing the view stack, details dump, and editor-form views. Domain untouched.
tags: [issue, tui, redesign]
timestamp: 2026-09-13T00:00:00Z
---

# ISSUE 0073: TUI compositor redesign

- **Type**: task
- **Status**: open
- **Priority**: high
- **Labels**: [tui, redesign]
- **Assignee**: none
- **Related**: [0057](0057-dotdrift-tui-design-map.md), [0062](0062-tui-information-architecture.md), [0063](0063-two-pane-shell-prototype.md), [0065](0065-full-schema-editor-suite-design.md), [0072](0072-module-management-dialogs.md), [ADR-0003](../adr/0003-tui-shared-palette.md), [ADR-0008](../adr/0008-service-layer-doorway.md)
- **Related code**: [`internal/tui/`](../../internal/tui/)
- **Closing commits**: none

## Summary

Restructure `dotdrift tui`'s presentation layer into one shell (header /
nav / workspace / footer) with a modal compositor: overlays render as
modals on a stack instead of pushed views, the module view becomes a
sectioned, status-annotated workspace with inline field editing, sudo
elevation becomes a reason-listed modal, and `/` opens a fuzzy palette.
All domain packages and the service layer stay untouched.

## Details

Dogfooding the M14 shell surfaced structural problems the view-stack
architecture cannot fix incrementally:

- A sudo prompt can materialize in the footer of an unrelated view
  mid-keystroke — a credential prompt must be a place, not an
  interruption.
- The details pane is a text dump of `module.toml`; editing means leaving
  context for pushed form views, and the pushed-view stack makes `esc`
  semantics unpredictable per view.
- Overlays are invisible as first-class objects; there is no fast path to
  a known module.

The redesign (spec: [M15](../milestones/m15-tui-compositor.md)) keeps
every domain package, the 0065 draft machinery (file-scoped drafts,
three-tier validation, disk-conflict refusal), the apply session, and
ADR-0008's doorway. It replaces: the view stack (push/pop per view) with
a compositor (base + modal stack); the details dump with a sectioned
workspace surface; editor-form views with per-field inline editing over
the draft ledger; the footer sudo prompt with an elevation modal that
lists the privileged operations it covers; and adds a `/` fuzzy palette
over modules+layers, valid actions, and current-module fields.

Conventions that survive unchanged: pure bubbletea state machines with
golden `View()` tests (goldens become fixed-size composited frames),
styles resolved through the ADR-0003 palette registry with adaptive
`LightDark` colors, keys + mouse with keyboard primary, TUI consumes
`service.Service` through consumer-side narrow interfaces.

## Acceptance Criteria

- [ ] One shell: header / nav / workspace / footer; overlays are modals on
      a compositor stack; `esc` always means "get out of the topmost thing".
- [ ] Nav shows modules with overlay layers as expandable children, dirty
      markers, and placeholders for async loads.
- [ ] Workspace renders `module.toml` as a sectioned surface with status
      glyphs and layer tabs; every section has a designed empty state.
- [ ] Editing is per-field inline over the 0065 draft ledger; raw-text
      degraded mode for parse errors; save/discard with conflict refusal.
- [ ] Elevation is one modal per elevation with an explicit reason list;
      cancel aborts cleanly; password handled as zeroed bytes, never
      logged or drafted.
- [ ] `/` palette indexes modules+layers, contextually valid actions, and
      current-module fields with fzf-style ranking and in-session recents.
- [ ] Every destructive action goes through one confirm component
      (`y` confirms, everything else cancels, full target identity).
- [ ] Keymap is a single table: shift-for-dangerous, no undo, contextual
      `?` help.
- [ ] The old view stack and orphaned views are deleted; `go test ./...`,
      `go vet ./...`, `golangci-lint run ./...` green.

## Out of Scope

- Domain changes (config, resolve, plan, apply, detect, profiles, drafts).
- CLI output changes; non-interactive elevation behavior.
- Command palette beyond navigation scope (no shell commands, no file
  search outside the profile, no keybinding remapping).
- Undo, another-host preview, when-tree builder (0065-D12's fog list
  stands).

## Notes

The full section-by-section design (shell, nav, workspace, editing,
elevation, palette, dialogs, keymap+testing, execution strategy) was
reviewed and accepted section by section; M15's milestone file carries it
as the spec. Execution order: compositor first (it is the platform), old
view layer deleted last (both layers coexist during the migration so every
task stays green).
