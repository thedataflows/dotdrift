---
type: Issue
title: Configurable status color palette
description: Override the colored-output defaults per named color from dotdrift.toml [colors]; missing becomes light red.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0013: Configurable status color palette

- **Type**: feature
- **Status**: done
- **Priority**: low
- **Labels**: [status, cli, ux]
- **Assignee**: none
- **Related**: [ADR-0003 TUI palette](../adr/0003-tui-shared-palette.md), [cli-surface](../product/cli-surface.md)
- **Related code**: [`internal/palette/`](../../internal/palette/), [`internal/drift/`](../../internal/drift/), [`cmd/`](../../cmd/)
- **Closing commits**: pending

## Summary

Every colored-output role in `status`/`modules`/`plan`/`diff` gets a name, and `dotdrift.toml` can override the default color per role via a new `[colors]` table. Additionally, `missing` drift switches from orange to **light red** (and its faint/bold shades derive from it) — gone items should read as the strongest signal, one step below hard failures.

## Details

Roles (the single vocabulary for all colored CLI output; the value is an
ANSI SGR sequence such as `"31"` red or `"38;5;208"` orange):

| Role | Default | Used for |
|---|---|---|
| `ok` | `32` (green) | all-checks-passed lines, `no drift` |
| `missing` | `91` (**light red**, was orange) | missing/removed items — the strongest drift signal |
| `warn` | `33` (yellow) | content/version differs |
| `error` | `31` (red) | unknown, not-a-symlink |
| `orphan` | `35` (magenta) | orphans section |
| `dim` | `90` (bright black) | dimmed module/description text |

Configuration, validated at `dotdrift.toml` load:

```toml
[colors]
missing = "38;5;75"   # any SGR params without the CSI/ESC wrapper
orphan  = "94"
```

- Values are raw SGR parameter strings (`31`, `1;31`, `38;5;N`); dotdrift
  wraps them in ESC `[` … `m` at render time. This keeps the file
  terminal-agnostic (no escape codes pasted into TOML) while allowing
  256-color and bold/underline combinations.
- Unknown role names and malformed values (empty, containing ESC/CSI
  wrappers, non-SGR control bytes) are load-time errors naming the file
  and key — same fail-loud bar as module.toml.
- Overrides work from any `dotdrift.toml` layer (base/host/user);
  precedence follows the existing config union (higher layers win).
- `NO_COLOR` / `--no-color` still gate everything: overrides only change
  hues, never re-enable color.

Implementation: new `internal/palette` package — `Palette` struct with one
field per role, `Default()`, `FromConfig(cfg map[string]string)` with
validation, and `Wrap(role, s)` rendering. `profile.Config` gains
`Colors map[string]string` (`[colors]`), unioned across layers in
`unionConfig`. `drift.Render`/`ColorDiff` and `cmd/modules` take a
`*palette.Palette` (nil = defaults); `status`/`modules`/`plan` load it
from the profile.

Out of scope: `main.go`'s lipgloss bold-red error line (separate renderer,
colors fixed by the terminal profile) and the generate-TUI palette
(ADR-0003 keeps those fixed until a theme system lands).

## Acceptance Criteria

- [x] `[colors]` in any `dotdrift.toml` overrides per-role defaults; unknown keys/values are load errors.
- [x] `missing` findings render light red (plain `91`, bold `1;91` shade on TTY) instead of orange.
- [x] `NO_COLOR`/`--no-color` still fully disable color with overrides set.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- The generate wizard / TUI color scheme (ADR-0003).
- `main.go` fatal-error styling (lipgloss, one site).
- True-color/RGB themes and light-background detection.

## Notes

- Tests: `internal/palette/palette_test.go` (defaults, validation errors, wrap, overrides), drift render tests for the light-red missing hue and override plumbing, `TestProfile_colorsConfig` union/precedence.
