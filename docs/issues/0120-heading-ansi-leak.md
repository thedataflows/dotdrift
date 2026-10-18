---
type: Issue
title: Section heading leaks ANSI text
description: The underlined heading style wraps the styled add mark and mangles its escape sequence into literal text.
tags: [issue, tui]
timestamp: 2026-10-18T18:00:00Z
---

# ISSUE 0120: Section heading leaks ANSI text

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [tui, ux, regression]
- **Assignee**: agent
- **Related**: [0119](0119-section-headings-hairlines.md), [TUI specification](../product/tui.md)
- **Related code**: [TUI](../../internal/tui/)
- **Closing commits**: `6857272`

## Summary

Since 0119 every workspace section heading shows literal `[38;5;240m`
text before its add mark. Lipgloss v2 re-applies an underline style
rune by rune over nested ANSI sequences and drops the inner escape
byte, so a styled segment inside an underlined run prints as raw text.
The goldens pinned the mangled output, so the suite stayed green.

## Requirements

- No styled segment is rendered inside the underlined heading style:
  the heading name renders alone, the add mark outside the run, and
  the lead spaces outside the run (underlined spaces showed as ticks).
- A regression test asserts no mangled escape (`ESC ESC`) in the raw
  frame and no leaked `[38;` text in the stripped frame.
