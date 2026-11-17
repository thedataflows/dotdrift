---
type: Issue
title: System dotfiles keep their mode via dotfiles apply
description: System-scope whole-file entries converge through [dotfiles] with the declared mode preserved (symlink stays symlink), elevated like system edits, in dependency-level order. The [bootstrap.files] content-copy translation is deleted.
tags: [issue, dotfiles, scope, mise]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0156: System dotfiles keep their mode

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: []
- **Assignee**: none
- **Related**: [0042](0042-system-files-bootstrap-files.md), [0151](0151-dotfile-operations-create-parent-directories.md), [0154](0154-system-files-existing-parent-suppresses-sudo.md), [0155](0155-chained-system-files-level-order.md)
- **Related code**: [`internal/service/`](../../internal/service/), [`internal/mise/`](../../internal/mise/)
- **Closing commits**: none

## Summary

The concept, stated by the user: a declared symlink means mise creates a symlink from source to target. dotdrift does not check in advance whether a source exists, and absolute sources are not its concern. When system scope runs elevated, order is essential because entries can depend on each other.

The [bootstrap.files] translation (issue 0042) breaks the first point: bootstrap.files cannot express a symlink (verified in issue 0155 against the current mise docs), so system symlink entries silently became content copies. System whole-file entries therefore converge through [dotfiles] instead — the same mechanism as user scope and system edits — with the declared mode preserved.

## Details

- The system files step emits one [dotfiles] config for its whole-file and edit entries, modes verbatim, and applies it per dependency level (0155 grouping, now over dotfile entries): an entry whose source is another entry's target, or lies under it, applies in a later pass, because a copy or template reads its source. A cycle is an error naming the targets.
- Elevation: when any entry target is not user-writable (and euid is not 0), every pass runs elevated through the handover seam (sudo -E mise dotfiles apply), like system edits today; the step classifies for the TUI sudo gate on the same predicate. When everything is writable, passes run unelevated.
- Declared secrets ride the same config: current mise reads [bootstrap.secrets] values in [dotfiles] templates too (mise bootstrap docs), so system templates keep secret().
- Missing target parents and mount destinations still ride a directories-only mise bootstrap --only files pass first, under the 0154 rule (only missing parents are declared). [bootstrap.files] is no longer generated for dotfiles: GenerateBootstrapFiles and OrderBootstrapFiles are deleted. ResolveBootstrapFiles stays for drift and diff probes.
- dotdrift does not existence-check sources for this path: relative sources resolve through the layers as before (that is how shadowing works), absolute sources pass through (0153), and symlink-each no longer reads its source directory in the step.

## Acceptance Criteria

- [x] A system entry with mode = "symlink" lands in the applied config as mode = "symlink" with its declared source; nothing rewrites it to copy.
- [x] Chained system entries apply in dependency order, one dotfiles apply pass per level; a cycle errors naming the targets.
- [x] A system batch with a non-writable target runs elevated through the handover and classifies for the sudo gate; an all-writable batch runs unelevated.
- [x] Mount destinations and missing parents still converge via [bootstrap.directories]; system templates keep declared secrets in the applied config.
- [x] GenerateBootstrapFiles and OrderBootstrapFiles are deleted; contract, README, and log describe the new mechanism; issue 0042 is marked superseded for whole-file entries.

## Out of Scope

- User-scope dotfiles: unchanged (already [dotfiles], already ordered by mise). Level ordering applies to the system step, where dotdrift controls the elevated passes.
