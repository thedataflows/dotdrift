---
type: Milestone
title: M6 Mise dotfiles
description: Generate dotfiles entries; apply; stop on conflict.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 6
---

# Goal

Dotfiles step only via mise; conflict prints guidance and saves resume state.

# Exit criteria

- No auto `--force`
- Fake conflict stops pipeline
- Full fake pipeline resolve→…→hooks green

# Tasks

1. [T6 dotfiles](/tasks/t6-dotfiles.md)

# Depends on

[M5](m5-mise-tools.md)