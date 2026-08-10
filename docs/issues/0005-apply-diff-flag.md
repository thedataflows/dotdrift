# ISSUE 0005: Apply --diff flag

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [apply, cli, ux]
- **Assignee**: none
- **Related**: [issue 0004](0004-resume-cursor-and-status-drift.md), [cli-surface](../product/cli-surface.md)
- **Related code**: [`internal/drift/`](../../internal/drift/), [`cmd/`](../../cmd/)
- **Closing commits**: pending

## Summary

`dotdrift apply` gains a `--diff` flag that displays colored unified diffs for files whose content differs between the profile source and the live target, before the apply pipeline runs. Bare `--diff` uses an internal Go diff library (`pmezard/go-difflib`, already vendored via testify); `--diff=<tool>` executes the named diff tool (e.g. `delta`, `difftastic`) as a subprocess with the two file paths.

## Details

The drift report (`status`) already identifies which files differ, but the user has no way to see *what* changed before applying. `apply --diff` bridges this gap: it iterates copy-mode dotfiles whose content differs, computes a unified diff (current → desired), and prints it — colored on TTY (green additions, red deletions, cyan hunk headers) — before the pipeline starts.

The flag form `--diff` (bare) means "use internal diff"; `--diff=delta` means "run `delta target source`". Since kong string flags require a value, bare `--diff` is normalized to `--diff=internal` by an arg-preprocessing loop in `cmd.Run` before kong parses. The sentinel `"internal"` cannot collide with a real tool name.

Initially scoped to `copy`-mode dotfiles with "content differs" — the most common case. Symlink "points to X", missing files, and other modes don't produce meaningful line diffs.

## Acceptance Criteria

- [x] `apply --diff` prints a unified diff for every copy-mode dotfile whose content differs, before the pipeline runs.
- [x] `apply --diff=delta` runs `delta <target> <source>` for each differing file.
- [x] Diff output is colored on TTY (green/red/cyan); plain when piped or `--no-color`.
- [x] No diff output when files are identical or target is missing.
- [x] `go test ./... -race` green; `go vet` clean.
- [x] `docs/log.md`, `docs/product/cli-surface.md` updated.

## Out of Scope

- Symlink "points to X" diffs (one-line target path change — not a content diff).
- Template-mode diffs (mise renders templates; recomputing is out of scope).
- Diff for packages/tools/mounts/smb (not file content).
- A `--diff` flag on `status` (status already shows drift summary; diff-on-status is a follow-up if dogfooding wants it).

## Notes

- **Library**: `github.com/pmezard/go-difflib/difflib` — already vendored as indirect dep via testify. Produces standard unified diff. Zero new dependencies.
- **Color**: ANSI codes (same constants as `drift.Render`); `--no-color` / `NO_COLOR` env suppresses via existing `executil.ColorEnabled` gate. External tools receive `NO_COLOR=1` env automatically.
- **External tool interface**: `tool target source` (two file paths as args). `diff` gets `-u` prepended automatically. Other tools (delta, difftastic) work with bare paths.
