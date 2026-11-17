---
type: Issue
title: Chained system files converge level by level
description: A system dotfile whose source is another entry's target fails in one bootstrap run, because mise reads every source while planning. Chained entries must converge in dependency-level order, one bootstrap run per level.
tags: [issue, dotfiles, bootstrap, scope]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0155: Chained system files converge level by level

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: []
- **Assignee**: none
- **Related**: [0042](0042-system-files-bootstrap-files.md), [0153](0153-absolute-dotfile-source-paths.md)
- **Related code**: [`internal/mise/`](../../internal/mise/), [`internal/service/`](../../internal/service/)
- **Closing commits**: none

## Summary

A module can declare a chain: one entry creates a file, a second entry uses that file as its source. Example (system scope):

    [dotfiles]
    "/root/.profile" = { source = "home/.profile", mode = "copy" }
    "/root/.bashrc" = { source = "/root/.profile", mode = "symlink" }

In one [bootstrap.files] run this fails: mise reads every source while it plans, before it applies anything, so /root/.profile does not exist yet when mise reads it for the .bashrc entry. Entry order inside the generated file does not help (verified live against mise 2026.11.8: both orders fail, and no file is created).

## Details

The system files step must group whole-file entries into dependency levels: an entry whose source is another entry's target (or lies under it) goes one level later. Each level converges in its own mise bootstrap --only files run, so the sources of a level exist when mise plans it. Directory and secret sections ride the first run's config (secrets ride every run: a template at any level can use them). A dependency cycle is an error naming the targets in the cycle.

Note on mode = "symlink": [bootstrap.files] has no symlink form — its mode field is an octal permission string. System-scope whole-file entries converge as content copies (issue 0042); a chained symlink entry therefore lands as a copy of the intermediate file's content, which is the same content a link would serve.

## Acceptance Criteria

- [x] Chained system entries converge: level 0 in the first bootstrap run, dependents in later runs, in dependency order.
- [x] A source-dependency cycle between system entries is an error naming the targets involved.
- [x] Unchained entries still converge in a single bootstrap run, with directories and secrets as before.
- [x] Tests pin the level grouping, the cycle error, and the per-level bootstrap runs.
- [x] Contract and log state the level rule.

## Out of Scope

- User-scope [dotfiles] chains (mise dotfiles apply). Same shape; separate question.
- Real symlinks for system whole-file entries. [bootstrap.files] cannot express them.

## Notes

Approach replaced by issue 0158 (2026-11-17): mise 2026.11.0 dotfiles depends converges a chained pair in one pass, verified live. The per-level passes below are deleted; the plan-time source check they worked around is unchanged.

Verified live with mise 2026.11.8: a two-entry chain in one config fails at plan time ('failed to read source ... No such file or directory') in both entry orders, creating nothing; mode = "symlink" in a bootstrap.files entry is rejected ('mode must be an octal string').

Online check (2026-11-17, latest mise 2026.11.9): the [bootstrap.files reference](https://mise.jdx.dev/bootstrap/files.html) lists every entry key — source, content, template, remove_empty, owner, group, mode (octal string), state, recursive, replace, phase, os, notify. There is no symlink or link key; the page mentions symlinks only to say mise does not follow them. Symlinks exist only in [dotfiles] (mode = "symlink"), the user-scope mechanism.
