---
type: Issue
title: "0145: View bodies must fill the frame (right padding, Profile bottom)"
description: Every view body stops short of the right edge (panes render 2 chars narrower than their slot, 4 lost per frame), and the Profile view's short body lets the footer float up instead of staying at the bottom.
tags: [issue, bug, tui]
timestamp: 2026-10-18T00:00:00Z
---

# ISSUE 0145: View bodies must fill the frame (right padding, Profile bottom)

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui]
- **Assignee**: none
- **Related**: [0144](0144-profile-view-outer-card.md)
- **Related code**: [`internal/tui/compositor.go`](../../internal/tui/compositor.go), [`internal/tui/views.go`](../../internal/tui/views.go), [`internal/tui/profileview.go`](../../internal/tui/profileview.go)
- **Closing commits**: `bafa417`

## Summary

User report: there is still too much padding on the right in all
views, and in the Profile view (4) the bottom info must stay at the
bottom.

## Details

Measured on a 100-char frame: view bodies render 96 chars wide.
Every pane is built 2 chars narrower than its slot
(`pane(navW-2, ...)` with content at `navW-4`), so a two-pane body
loses 4 chars on the right and the Profile cards (0144) stop 2
short. Fix: panes take their full slot width and their content the
full inner width (slot minus the 2 border cells) — Modules,
Status, and Plan alike; the Profile cards stretch the full frame
width (the 0144 width-2 gives way to flush, per this report).

The Profile body also sizes naturally since 0144 removed its pane,
so at tall frames the footer (the bottom info line) floats up
under the cards. The body pads its lines to the pane height, as
the pane border used to enforce, and the footer stays at the
bottom. Content taller than the pane still clips.

## Acceptance Criteria

- [x] Two-pane bodies span the full frame width; content reaches
  the panes' inner right edge.
- [x] Profile cards span the full frame width, and the Profile
  body fills the pane height so the footer stays at the bottom.
- [x] Goldens are regenerated and reviewed; frames still fit the
  terminal at small sizes.

## Out of Scope

- Modal widths and the panes' left-side layout.
