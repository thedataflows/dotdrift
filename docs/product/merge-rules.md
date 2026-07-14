---
type: Specification
title: Merge rules
description: Layer precedence for host and user overlays.
tags: [product, merge]
timestamp: 2026-07-14T00:00:00Z
---

# Precedence (low → high)

1. `modules/<id>/` (base)
2. `hosts/<hostname>/`
3. `users/<username>/` — **wins on conflicts**

# Rules

| Kind | Resolution |
|------|------------|
| Dotfiles target path | User entry wins; lower layers dropped for that target |
| File tree same relative path | User > host > module |
| Packages present/absent | Higher layer wins; absent can cancel present |
| Tools map keys | Higher layer wins |
| `modules.disable` | **Union** across layers (any disable sticks; no silent re-enable in v0.1) |
| `when` filters | Evaluated on module; fail → not selected |

# Plan visibility

`dotdrift plan` and `dotdrift modules` SHOULD show winning layer for overridden targets.