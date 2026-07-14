---
type: Milestone
title: M4 Packages
description: Paru/pacman backend integrated into apply.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 4
---

# Goal

Install/remove packages for selected modules.

# Exit criteria

- Backend interface + fake + paru argv tests
- Idempotent present (`--needed`)
- Sudo may be interactive

# Tasks

1. [T4 packages](/tasks/t4-packages.md)

# Depends on

[M3](m3-state-apply.md)