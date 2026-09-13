---
type: Task
title: T-tui-workspace
description: The workspace read surface — module.toml as a sectioned, status-annotated surface with layer tabs, designed empty states, and a raw-text parse-error mode.
tags: [task, tdd, tui, workspace]
timestamp: 2026-09-13T00:00:00Z
milestone: m15
---

# Goal

Replace the M14 details text dump with the sectioned workspace surface
per [M15](/milestones/m15-tui-compositor.md): `module.toml` rendered as
structured sections with status glyphs, layer tabs synced with the nav,
and graceful degradation. Read-only here; editing is T-tui-editing.

# Tests first

- Golden `View()` per section and state: `TestWorkspace_allSections` —
  meta, packages, links, writes, when, hooks, systemd.units, tools — each
  populated and each empty (`TestWorkspace_emptyStatesDesigned`, never a
  blank pane); `TestWorkspace_statusGlyphs` (including the `needs root`
  Warning marker fed by plan results);
  `TestWorkspace_layerTabsWithAndWithoutOverlays`;
  `TestWorkspace_parseErrorShowsRawText` (broken file opens read-only,
  error named, raw text visible — 0065 behavior preserved);
  `TestWorkspace_loadPlaceholder`.
- Message-driven: `TestWorkspace_layerTabCycles` (base → user → host,
  `L`), `TestWorkspace_layerTabsSyncWithNav` (selecting an overlay child
  in nav moves the tab and vice versa),
  `TestWorkspace_selectionFollowsNav` (module switch swaps the whole
  surface).

# Implementation notes

- Rendering consumes the config area's strict decode + raw text (the
  0065 seam); the workspace never parses TOML itself.
- `needs root` markers come from the plan result's privileged-operation
  set — the TUI renders the elevation question, the domain decides it.
- Section order is fixed and matches the schema docs; unknown sections
  render in a trailing "other" group rather than being dropped.
- The surface is scrollable; cursor movement (`j`/`k`) walks rows so
  T-tui-editing can hang edit mode off the same cursor.

# Docs

- tui.md workspace section rewritten (details dump description deleted);
  log entry.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
