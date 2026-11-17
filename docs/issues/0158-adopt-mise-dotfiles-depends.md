---
type: Issue
title: Adopt mise dotfiles depends and raise the mise floor to 2026.11.9
description: Implementation plan to use dotfiles depends for chained entries, write min_version into generated configs, and take the 2026.11 dotfiles safety fixes.
tags: [mise, dotfiles, apply]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0158: Adopt mise dotfiles depends and raise the mise floor

- **Type**: task
- **Status**: done
- **Priority**: high
- **Labels**: [mise, dotfiles]
- **Assignee**: none
- **Related**: [0155](0155-chained-system-files-level-order.md), [0156](0156-system-dotfiles-keep-mode-dotfiles-apply.md), [0157](0157-elevated-mise-root-state.md), [contract invariant 13](../product/contract.md)
- **Related code**: [internal/mise/](../../internal/mise/), [internal/service/steps.go](../../internal/service/steps.go)
- **Closing commits**: none

## Summary

mise 2026.11.0 lets a whole-file dotfiles entry declare depends on other
entries in the same apply. Use it for chained entries, write min_version
into every generated mise config, and raise the mise floor to 2026.11.9 so
applies get the 2026.11 dotfiles safety fixes.

## Details

Today a chained entry set, where one entry source is another entry target,
converges in one apply pass per dependency level (issue 0155, generalized
in 0156). mise reads or checks every source when it plans an apply, so a
single config with both entries fails before anything is written. The
level passes in systemFilesStep are dotdrift code that exists only for
this limit. User scope has no levels at all, so a chained user entry set
fails in one pass today.

mise 2026.11.0 adds a depends key to whole-file dotfiles entries. mise
writes the named entries before it renders or reads the dependent entry.
The key names groups or whole-file entries by target path. It does not
work for edit entries or track modes. Source: the mise dotfiles
reference, checked 2026-11-17 against mise 2026.11.9.

The same release range carries safety fixes dotdrift gets for free by
raising the floor: 2026.11.7 refuses to overwrite an edited copy when
there is no terminal, 2026.11.8 warns when a copy target was edited since
mise wrote it and skips unchanged files in a directory copy, 2026.11.9
warns about config keys no entry reads and refuses whole-file keys on
edit entries. An old mise also warns about and skips an entry with an
unknown mode instead of failing, so generated configs must state their
own floor with min_version. dotdrift checks the version in Go, but the
elevated child and a hand run of mise in a profile directory do not pass
that check again.

## Implementation plan

Step 0 is a gate, not code. With real mise in a throwaway directory,
apply one dotfiles config with a copy entry and a symlink entry whose
source is the copy target, the symlink entry declaring depends on the
copy target. If the pair converges in one apply, depends replaces the
levels. If mise still reads the chained source at plan time, stop: keep
the levels, record the live result in this issue, and do only steps 1
and 2. Do not delete working code on a docs claim.

1. Raise MinMiseVersion in internal/mise/mise.go from 2026.8.2 to
   2026.11.9. The ensure path already upgrades a user install and names
   the package manager for a system install.
2. Write min_version = "2026.11.9" at the top of every mise config
   dotdrift generates: the shared apply config, the packages config, the
   tools fragments, the user and system dotfiles configs, the
   directories-only bootstrap config, and the systemd, mounts and smb
   bootstrap configs. One shared prefix constant in internal/mise, used
   by every generator or config write site. Acceptance tests assert the
   line in each generated config.
3. Emit depends in GenerateDotfiles. An entry whose expanded source is
   another entry target in the same generated config, or lies under a
   symlink-each target in it, declares depends on that target. Edit
   entries never declare it and never satisfy it. Keep the emission
   deterministic: sorted, deduplicated. A dependency cycle, including an
   entry that sources itself, is a resolve or generate error that names
   the targets, as the level code does today.
4. With step 0 green, delete the level passes. systemFilesStep writes
   one dotfiles config for all system entries and applies it once,
   elevated when a target is not user-writable, as today. The user
   DotfilesStep needs no change beyond the depends lines: chained user
   entries start working. Keep the directories-only bootstrap pass for
   mount destinations and missing parents (issue 0151, 0154). Keep the
   elevated environment anchoring (issue 0157).
5. Update contract invariant 13 and docs/log.md. Keep invariant wording
   at the behavior level: chained whole-file entries converge in one
   apply, ordered by mise depends.

Tests, red first, one behavior at a time:

- GenerateDotfiles emits depends for a chained pair, in the copy then
  symlink order and in the reverse declaration order.
- GenerateDotfiles emits no depends for edit entries, and a cycle errors
  with the targets named.
- Every generated config carries the min_version line.
- MinMiseVersion is 2026.11.9, through the existing constant test seam.
- The system files step applies chained entries in one dotfiles apply
  pass. The per-level test from issue 0155 is replaced, not edited to
  pass: capture the applied config and assert one pass with depends.
- A user scope chained pair converges through DotfilesStep with a fake
  runner that records the config.

## Acceptance Criteria

- [x] Step 0 live result is recorded in this issue, pass or fail
- [x] MinMiseVersion is 2026.11.9 and every generated mise config carries min_version = "2026.11.9"
- [x] A chained whole-file pair converges in one dotfiles apply pass at user scope and at system scope, with depends in the applied config
- [x] Edit entries declare no depends; a source dependency cycle errors and names the targets
- [x] Missing parents and mount destinations still get the directories-only bootstrap pass
- [x] go test ./... and go vet ./... pass

## Out of Scope

- Moving system block edits to bootstrap.files. Separate spike: it gains
  owner, group, mode, phase and notify, but covers block edits only.
- New module.toml keys: merge edits, content entries, permissions-only
  entries, mode fnox. Each needs its own issue and user demand.
- Task report files, task locks, daemons, dotfiles history or track
  modes. Reviewed 2026-11-17 and rejected for this project.

## Notes

Step 0 result, 2026-11-17, real mise 2026.11.8 in a throwaway directory:
with depends on the symlink entry, one dotfiles apply copied the profile
and created the symlink to it. The same config without depends failed at
plan time with sources do not exist and created nothing. The gate
passes: depends replaces the level passes for whole-file entries.

Research basis: the mise changelog and docs pages for glossary,
dotfiles, the dotfiles reference, configuration, and the configuration
reference, fetched 2026-11-17. Latest mise is 2026.11.9 (2026-11-16);
the machine mise is 2026.11.8.
