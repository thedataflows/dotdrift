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

`<username>` is the OS account name (`os/user.Current()`), never `$USER`.
Under `sudo dotdrift apply` the process account is root, so **root's overlays
are selected**, not the invoking user's.

# Rules

| Kind | Resolution |
|------|------------|
| Dotfiles target path | User entry wins; lower layers dropped for that target |
| File tree same relative path | User > host > module |
| Packages present/absent | Higher layer wins; absent can cancel present |
| Tools map keys | Higher layer wins |
| `modules.disable` | **Union** across layers (any disable sticks; no silent re-enable in v0.1) |
| `when` filters | Evaluated on module; fail → not selected |

`when` filter values match detected facts by **case-sensitive exact match**
(`when.distro: Arch` never matches detected `arch`). GPU detection is
**first-match** over `lspci` output (nvidia → amd → intel), and `when.gpu` is
a **single string**, not a list.

# Plan visibility

`dotdrift plan` and `dotdrift modules` SHOULD show winning layer for overridden targets.