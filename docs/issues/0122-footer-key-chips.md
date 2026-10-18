---
type: Issue
title: Footer keys as deliberate chips
description: Footer key hints share the bar background but render bold white, so they read as broken black boxes; give them an intentional indigo chip.
tags: [issue, tui]
timestamp: 2026-10-18T18:20:00Z
---

# ISSUE 0122: Footer keys as deliberate chips

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, ux]
- **Assignee**: agent
- **Related**: [0113](0113-tui-visual-system.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `665bc5e`

## Summary

In the terminal the footer keys (`enter`, `a`, `d`, `ctrl+s`, …) read
as black boxes punched into the bar. Pixel sampling of the capture
shows the bar background is in fact uniform: the keys share the bar
background but render bold pure-white, so each key pops as an
accidental dark chip against its muted description. gh-dash key hints
are deliberate chips — own background, padding, primary text.

## Requirements

- A footer key renders as an indigo chip with horizontal padding,
  visually distinct from the bar background.
- Descriptions and separators keep the bar background.
- A test pins that the key background differs from the bar background.
