---
type: Issue
title: Elevated mise must not write the invoking user's state
description: The elevated dotfiles child runs sudo -E mise with the user's HOME and XDG dirs, so root's mise writes its history and state into ~/.local/state and fails there. The elevated child gets root's home-anchored dirs.
tags: [issue, dotfiles, scope, mise]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0157: Elevated mise uses root's state, not the user's

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: []
- **Assignee**: none
- **Related**: [0156](0156-system-dotfiles-keep-mode-dotfiles-apply.md), [0071](../archive/README.md)
- **Related code**: [`internal/mise/`](../../internal/mise/)
- **Closing commits**: none

## Summary

Running an elevated system batch fails: mise ERROR failed read_to_string: ~/.local/state/mise/history/index/checkpoints.json — Permission denied. The elevated child is sudo -E mise: -E preserves the invoking user's HOME and XDG dirs, so the root mise process anchors its history and state in the user's ~/.local/state, where it must not write (and where root-owned leftovers would then break the user's own mise). The elevated parts are for system actions only; mise's own bookkeeping belongs to root.

## Details

DotfilesApplySudoCmd builds the only elevated mise child. In its sudo branch, the child env now overrides the home-anchored locations with root's: HOME, XDG_STATE_HOME, XDG_DATA_HOME, XDG_CACHE_HOME, XDG_CONFIG_HOME, and mise's own MISE_STATE_DIR, MISE_DATA_DIR, MISE_CACHE_DIR, MISE_CONFIG_DIR. Root's home comes from the root account lookup, falling back to /root. The already-root branch (euid 0) is unchanged: that process already runs with root's environment. The trust plumbing (MISE_TRUSTED_CONFIG_PATHS) is untouched.

## Acceptance Criteria

- [x] The elevated (sudo) child's environment anchors HOME and the XDG/MISE state, data, cache, and config dirs at root's home, not the invoking user's.
- [x] The already-root child's environment is unchanged.
- [x] Tests pin both branches.

## Out of Scope

- Other sudo children (smb's pdbedit/smbpasswd): not mise, no mise state.
