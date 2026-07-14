---
type: Task
title: T1 Profile and modules
description: TOML load, discover, selection, modules command.
tags: [task, tdd]
timestamp: 2026-07-14T00:00:00Z
milestone: m1
---

# Goal

Implement [profile layout](/product/profile-layout.md) loading and selection.

# Tests first

- `TestLoadDotdriftTOML_*`
- `TestLoadModuleTOML_*`
- `TestDiscoverModules_*`
- `TestSelection_presenceMeansEnabled`
- `TestSelection_disableUnion`
- `TestSelection_whenFilter`
- `TestCLI_modules_listsStatus` (golden)

# Acceptance

- No enable list in schema
- `dotdrift modules` is list (not `modules ls`)