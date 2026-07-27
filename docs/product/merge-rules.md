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
| Module `scope` | **Representative layer only** (base preferred by discovery order); overlay `scope` declarations are ignored |
| `mounts.<name>` | **Whole-entry by name**: higher layer's entry fully replaces the lower layer's (like dotfiles per-target); no field-level merge |
| `smb.shares.<name>` | **Whole-entry by name**, same as mounts |
| `smb` scalars (`group`, `users`, `avahi`) | **Replace when set**: a higher layer's non-empty value (or present `avahi` key) replaces; unset keys leave the lower value. `users` replaces wholesale — never appended (contrast: hooks append) |

# Overlay-only modules (ADR-0001)

Presence = managed holds per layer: a module that exists only under
`hosts/<hostname>/modules/<dir>/` or `users/<username>/modules/<dir>/` is
selected like a base module. During merge the absent layers are treated as
empty, so packages, tools, dotfiles, and hooks resolve from whichever layers
exist. Layer directories key on the module **directory name**, never the
declared `id`: the user overlay for `hosts/<h>/modules/foo/` lives at
`users/<u>/modules/foo/` regardless of the `id` in its `module.toml`.

The representative module config (the first layer in base → host → user
order containing the directory) supplies module-level metadata such as
`scope`, `id`, `app`, and `when`; overlays merge only packages, tools,
dotfiles, hooks, mounts, and smb.

`when` filter values match detected facts by **case-sensitive exact match**
(`when.distro: Arch` never matches detected `arch`). GPU detection is
**first-match** over `lspci` output (nvidia → amd → intel), and `when.gpu` is
a **single string**, not a list.

# Plan visibility

`dotdrift plan` and `dotdrift modules` SHOULD show winning layer for overridden targets.