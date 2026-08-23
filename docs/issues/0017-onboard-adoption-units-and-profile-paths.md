---
type: Issue
title: Onboard adoption claims giant ancestors and mangles profile-internal paths
description: "would adopt: ~/.config (home/.config)" from a host-layer orphan, and passing a module file path as the live path produces a garbage ~/dotfiles target.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0017: Onboard adoption claims giant ancestors and mangles profile-internal paths

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [onboard, orphans, profile]
- **Assignee**: none
- **Related**: [issue 0015](0015-onboard-adopts-orphans.md), [issue 0016](0016-status-orphans-current-host-only.md)
- **Related code**: [`internal/onboard/onboard.go`](../../internal/onboard/onboard.go)
- **Closing commits**: pending

## Summary

Field report: `onboard --host <profile>/hosts/cri-pc/modules/easyeffects/home/.config/easyeffects/db/easyeffectsrc --app=easyeffects --dry-run` answered `would adopt: ~/.config (home/.config)` — adopting the entire `~/.config` for one stray preset file. Two defects:

1. **Unbounded ancestor collapse.** A host layer holding one orphan file
   makes every ancestor dir "fully orphan" in-module; the topmost
   non-nesting one (`home/.config` -> `~/.config`) gets claimed. Claims
   were read only from the layer being written, so the base module's
   `~/.config/easyeffects` symlink-each entry never bounded the chain.
2. **Profile-internal paths treated as live paths.** Onboarding a path
   that IS a module file (inside the profile) maps it to a garbage
   target (`~/dotfiles/hosts/...`) and duplicates the tree under
   `home/dotfiles/...`. The intent — "add this orphaned file to the
   corresponding module.toml" — should be detected and answered with an
   adoption of that exact file, naming the layer (base/host/user).

## Details

- Claims (occupied `[dotfiles]` targets) merge across ALL of the
  module's layers (base, current host, current user), so a base
  declaration bounds every adoption chain.
- A DIRECTORY unit additionally never claims a shared namespace root
  (`~`, `~/.config`, `~/.local`, `~/.cache`, `/`, `/etc`, `/usr`,
  `/var`); FILE units are blocked only by an exact target duplicate, so
  a stray file under another entry's subtree is still adoptable under
  its own path.
- A path inside a module layer directory is a **directed adoption**:
  its layer becomes the target module (overriding `--app`/`--host`
  inference), no copy happens, and the notice names the level:
  `adopted: <target> (<source>) [base|host|user]`.
- Adoption references come from `drift.ReferencedPaths` (issue 0016) so
  onboard and status agree on what is an orphan.

## Acceptance Criteria

- [x] The easyeffects command answers with the file:
      `would adopt: ~/.config/easyeffects/db/easyeffectsrc (home/.config/easyeffects/db/easyeffectsrc) [host]`.
- [x] No adoption ever claims `~/.config` or another shared root; a
      deep orphan with no nesting bound adopts its own path, not an
      ancestor.
- [x] A profile-internal path produces an entry in ITS layer's
      module.toml with the correct target; no `~/dotfiles/...` target,
      no duplicated tree, file content untouched.
- [x] Adoption notices name the layer.

## Out of Scope

- Merging symlink-each source trees across layers (separate feature).
- Adopting into user layers via flags (directed paths reach them;
  `--user` does not exist).

## Notes

Blocked dirs list is a deliberate ceiling, marked `ponytail:` — the
upgrade path is configurability if real profiles need it.
