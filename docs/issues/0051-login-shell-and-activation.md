---
type: Issue
title: "TICKET: login shell and mise shell activation"
description: Decision ticket — adopt [bootstrap.user].login_shell and/or [bootstrap.mise_shell_activate] as module.toml sections, and their interplay with existing dotfile rc edits.
tags: [issue, wayfinder:grilling, mise, shell]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0051: TICKET — login shell and mise shell activation

- **Type**: feature
- **Status**: open
- **Priority**: low
- **Labels**: [wayfinder:grilling, mise, shell]
- **Assignee**: none
- **Related**: [map 0047](0047-mise-bootstrap-adoptions-map.md), [alignment B-shell](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/profile/profile.go`](../../internal/profile/profile.go), [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: none

## Question

Should dotdrift adopt mise's two shell conveniences, and if so, as one
section or two?

**Login shell** — `[bootstrap.user].login_shell = "/bin/zsh"`: chsh with
`/etc/shells` handling; under sudo targets `SUDO_USER`. One key. Question:
which module declares it (a `shell` module?), and is it worth a section for
a value most users set once — or rule out as out-of-band?

**Shell activation** — `[bootstrap.mise_shell_activate]`: marker-owned rc
blocks (`activate`/`shims` modes) for bash/zsh/fish. Open decisions:

1. **Overlap with existing config** — the dogfood profile already manages rc
   files and/or dotfile edit blocks for activation; mise skips generated
   blocks when `[dotfiles]` claims the same rc file ("explicit dotfiles
   win"). Does adoption simplify the profile or duplicate machinery?
2. **Relationship to the 0037 fragment** — the conf.d fragment activates
   *tools on PATH* globally; shell activation installs the *hook*. Both?
   Verify the profile's current activation path before deciding.
3. **Schema** — one `[shell]` module section with `activate = [...]` /
   `shims = [...]`, or passthrough of mise's per-file keys (zprofile, zshrc,
   bashrc, fish, …)?

Both halves may legitimately resolve as "rule out" — the ticket is the
decision, not the implementation.
