---
type: Issue
title: Saving a module.toml does not guarantee a trailing newline
description: WriteModuleLayer writes the spliced or raw candidate bytes verbatim; tomlsplice preserves the file's prior final-newline state and the raw-repair path writes the draft text as-is, so a module.toml without a trailing newline keeps missing it across every save, and a fresh save from an empty baseline creates one without it.
tags: [bug, tui, service, dogfooded]
timestamp: 2026-09-21T00:00:00Z
---

# ISSUE 0086: Saving a module.toml does not guarantee a trailing newline

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [tui, service, dogfooded]
- **Assignee**: none
- **Related**: [0065-D7's save pipeline](../milestones/), [0087](0087-tui-remove-row-dead-on-structural-rows.md)
- **Related code**: [`internal/service/config.go`](../../internal/service/config.go) (`WriteModuleLayer`), [`internal/tomlsplice/`](../../internal/tomlsplice/)
- **Closing commits**: 66c0a50

## Summary

Saving a module.toml (TUI `ctrl+s`, the 0065 save pipeline) writes the
candidate bytes verbatim. `tomlsplice.Splice` deliberately preserves the
file's prior final-newline state, and the raw-repair path
(`SaveRequest.Raw`) writes the draft text as-is — so a file missing its
trailing newline keeps missing it forever, and a save from an empty
baseline (a layer with no module.toml yet) creates a file with no
trailing newline. Found dogfooding: editors and `git diff` complain.

## Details

`tomlsplice.Splice`'s verbatim contract ("the document ends in a newline
exactly when it did before", `joinLines`) is correct for a splicer — the
normalization belongs at the write boundary, not in text mechanics.

Audit of every module.toml write path:

- `ConfigArea.WriteModuleLayer` — the only offender (spliced and raw
  candidates both written verbatim).
- `ConfigArea.CreateModule` / `OverrideModule` — scaffolds end in `\n`.
- `onboard.mergeModuleTOML` — fresh writes end in `\n` (encoder blocks
  do); merges inherit the splice contract, and any file they inherit
  from was either encoder-written or fixed by this issue's save path.
- `generate.writeModuleConfig` — BurntSushi's encoder emits a trailing
  newline per line.

Fix shape: in `WriteModuleLayer`, after the candidate is final (spliced
or raw) and before the strict decode, normalize a non-empty candidate to
end in exactly one `\n`. `SaveResult.Raw`/`Hash` must reflect the
normalized bytes so the draft rebases onto what was actually written.

## Acceptance Criteria

- [x] A save whose baseline file lacks a trailing newline writes the file
  with one (splice path)
- [x] A raw-repair save whose candidate lacks a trailing newline writes
  the file with one (raw path)
- [x] A save from an empty baseline (no module.toml) creates the file
  with a trailing newline
- [x] `SaveResult.Raw` equals the bytes on disk (hash and rebase
  consistent)
- [x] Files already ending in a newline save byte-identical otherwise
  (no double newline, no other churn)

## Out of Scope

- Changing `tomlsplice.Splice`'s verbatim contract.
- Trailing-newline policy for generated units, dotfile payloads, or any
  non-module.toml file.
