---
type: Milestone
title: M8 Onboard
description: Copy live config into module; immediate mise apply; no enable flag.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 8
---

# Goal

Day-2 workflow to grow modules; EnsureMise then mise apply.

# Exit criteria

- Default link mode
- Copy then mise order asserted by an order-recording test
- Module presence selects app automatically
- Conflict keeps module files, fails command
- Generated mise config lives under the XDG state dir with absolute dotfile sources; the profile directory carries no runtime files
- `--yes` propagates to `mise dotfiles apply`; file modes preserved when materializing paths (ownership is not)

# Tasks

1. [T8 onboard](/tasks/t8-onboard.md)

# Depends on

[M6](m6-mise-dotfiles.md), [M7](m7-detect.md)