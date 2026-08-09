---
type: Issue
title: paru mise package plugin
description: A dotdrift-backed mise package-manager plugin so Arch/AUR packages install via paru instead of mise's plain pacman built-in.
tags: [issue, feature, mise, packages, arch]
timestamp: 2026-08-05T00:00:00Z
---

# ISSUE 0003: paru mise package plugin

- **Type**: feature
- **Status**: done
- **Priority**: high
- **Labels**: [mise, packages, arch, plugin]
- **Assignee**: none
- **Related**: [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [issue 0002](0002-delegate-convergence-to-mise-bootstrap.md), [mise package plugin spec](https://mise.jdx.dev/package-plugin-development.html)
- **Related code**: [`internal/paru/`](../../internal/paru/) (plugin embed + lifecycle), [`internal/mise/`](../../internal/mise/), [`internal/packages/`](../../internal/packages/)
- **Closing commits**: bb1f6b6, f69e17e, b35eafe, 8a76c67

## Summary

dotdrift provides a mise package-manager plugin named `paru` so that Arch
packages (including AUR) install via `paru -S` instead of mise's plain
`pacman:` built-in. This preserves AUR support inside the mise convergence
model — a prerequisite for eventually trimming `internal/packages` (issue 0002,
tension 1), which is retained today only for removal (`Absent`) and
`plan --deps`.

## Details

mise's `[bootstrap.packages]` `pacman:` built-in-manager is plain pacman — no
AUR. dotdrift's Arch backend is `paru -S` (pacman + AUR), and the migration
doc references AUR packages (`ntfsprogs-plus`). mise package plugins
([spec](https://mise.jdx.dev/package-plugin-development.html)) are the
extension point: a Lua-based vfox plugin with batch
`PackageInstalled`/`PackageInstall` hooks; dotdrift copies the plugin into
mise's registry as a normal installed plugin and keys packages as
`paru:<pkg>` in `[bootstrap.packages]`.

### Design

Two parts: an embedded Lua shim plugin (protocol adapter, sourced from real
files under `internal/paru/miseplugin/` via `go:embed`) and a `dotdrift paru`
subcommand (all paru invocation logic in Go).

**Lua plugin** — copied into mise's registry at
`$XDG_DATA_HOME/mise/plugins/paru/` (the same on-disk shape as any other mise
plugin, not a symlink into a dotdrift-specific path):

```
paru/
├── metadata.lua            # assigns global PLUGIN = { name = "paru", … } (vfox convention)
├── mise.plugin.toml        # [package-manager] requires = ["paru"], os = ["linux"]
└── hooks/
    ├── package_installed.lua   # ctx → dotdrift paru installed → {name,state,version}[]
    └── package_install.lua     # ctx → dotdrift paru install [--dry-run] [--update]
```

`metadata.lua` must assign the global `PLUGIN` (not `return` a table): mise's
loader runs `require "metadata"; return PLUGIN`, so a `return`-style file
leaves `PLUGIN` nil and the hooks never attach. Each hook calls
`dotdrift paru <sub>` and parses the line-based status protocol into the vfox
response table; no paru logic lives in Lua.

**`dotdrift paru` subcommand** (Go):

| Subcommand | Behavior |
|---|---|
| `dotdrift paru installed` | Checks each name via `pacman -Q`; emits `name<TAB>state<TAB>version` lines. Side-effect free, never elevates. |
| `dotdrift paru install` | Runs `paru -S --needed --noconfirm <names>`. Honors `--dry-run` (print, don't run) and `--update`. |

**Resolver integration** (issue 0002): bare Arch package names translate to
`paru:<pkg>` keys; the `aur/<pkg>` marker is stripped to `paru:<pkg>`; an
explicit `manager:<pkg>` (colon) prefix passes through unchanged. On Arch, bare
names default to `paru` — not mise's AUR-less `pacman:` built-in.

**Lifecycle** (`paru.EnsureInstalled`, called on every Arch apply): dotdrift
hashes the installed plugin and compares it to the embedded plugin's SHA-256,
recopying the real directory only on mismatch, a missing plugin, or a stale
symlink — no writes on the common up-to-date path. Because the plugin is a
normal installed plugin in mise's registry, there is no `[bootstrap.plugins]`
declaration and the apply runs only `mise bootstrap --only packages`.

### Privilege: not a contract issue

The plugin hard contract — *"package plugins must never invoke `sudo` in any
hook; mise never elevates for them"* — constrains the **plugin's own code**
and mise's elevation, not the wrapped tool. paru is a user-space AUR helper
that self-elevates: it calls `sudo pacman -U` internally. The plugin (via
`dotdrift paru`) runs `paru`; the plugin's code path contains no `sudo` and
asks mise for none. This is architecturally identical to how built-in
managers work when mise elevates them — except paru owns its own elevation.
No contract violation.

The only practical concern was **TTY availability**: paru's password prompt
needs a controlling terminal. **Verified** on a real CachyOS host with mise
v2026.8.3: `mise bootstrap packages apply` connects the plugin subprocess to a
TTY under an interactive shell, so paru builds the package and `sudo pacman -U`
prompts normally; the `pre-packages` hook fallback was not needed. (A non-TTY
shell reproduces `sudo: a terminal is required` — the caller's environment, not
a mise limitation.)

## Acceptance Criteria

- [x] `dotdrift paru installed` correctly reports installed/missing for repo
      and AUR packages via `pacman -Q` (side-effect free, no elevation).
- [x] `dotdrift paru install` runs `paru -S --needed --noconfirm`, honoring
      `--dry-run` and `--update`.
- [x] The Lua shim plugin delegates to the subcommand and returns the vfox
      response shape (`{name, state, version?}` per package).
- [x] **metadata.lua assigns the global `PLUGIN`** (vfox convention
      `PLUGIN = { name = "paru", ... }`, not `return {...}`). mise's vfox
      loader runs `require "metadata"; return PLUGIN`; a metadata.lua that
      only `return`s a table leaves `PLUGIN` nil, so the package hooks never
      attach and `mise bootstrap plugins status` reports the plugin `missing`
      — the source of the contradictory "declared in [bootstrap.plugins] but
      not installed" warnings. Validated: status flips `missing`→`installed`
      and `packages status` recognizes `paru:<pkg>`.
- [x] **TTY validation:** on a real CachyOS host with mise v2026.8.3,
      `mise bootstrap packages apply` with a `paru:aur/fresh-editor-bin` entry
      installs the AUR package end-to-end — paru builds the package and
      `sudo pacman -U` reaches the TTY (verified under an interactive PTY;
      a non-TTY shell reproduces `sudo: a terminal is required`, which is the
      caller's environment, not a mise limitation). The `pre-packages` hook
      fallback was therefore **not** needed.
- [x] dotdrift maintains the plugin: `paru.EnsureInstalled` copies the embedded
      plugin into mise's registry (`$XDG_DATA_HOME/mise/plugins/paru/`) as real
      files (no symlink), recopying only on content-hash mismatch — no useless
      writes on the common path. The plugin source is `go:embed`'d from
      `internal/paru/miseplugin/`. It emits `paru:<pkg>` keys for bare Arch
      names and strips the `aur/<pkg>` marker (`aur/X` → `paru:X`) so
      `pacman -Q`/`paru -S` see the real package name.
- [x] `go test ./...` green; tests cover the `dotdrift paru` subcommand argv,
      the metadata.lua `PLUGIN =` assignment, `aur/` prefix stripping, the
      hash-gated copy (`copiesWhenMissing`/`noWriteWhenUpToDate`/
      `recopiesOnContentDrift`/`replacesSymlinkWithCopy`), and the exact
      embedded file set.

## Out of Scope

- Package removal/prune — mise's package-plugin v1 does not support uninstall
  (the spec reserves room for a future `PackageUninstall` hook); `packages.absent`
  handling moves to issue 0002's broader translation.
- Non-Arch platforms — only the `paru` manager; `apt`/`dnf` use mise built-ins.
- Version pins for AUR packages — `supports_version_pins = false` (AUR has no
  meaningful version pinning).

## Notes

The `requires = ["paru"]` in `mise.plugin.toml` means mise adds its shims and
global toolset bin paths to PATH but does not install paru. paru must be
pre-installed (or declared as a dotdrift-managed package/bootstrapped
separately); the e2e CachyOS image already ships paru from the CachyOS binary
repo.

`PackageInstalled` must be "side-effect free, fast, non-interactive, and never
elevate" — `pacman -Q` satisfies all four (read-only query, no sudo).
`PackageInstall` runs `paru -S`, which self-elevates internally; that is
paru's own behavior, not the plugin invoking sudo (see "Privilege: not a
contract issue" above).

**Plugin lifecycle is dotdrift-owned and hash-gated (`paru.EnsureInstalled`).**
The plugin lives in mise's registry as a real copied directory
(`$XDG_DATA_HOME/mise/plugins/paru/`) — the same on-disk shape as any other
mise plugin, no symlink into a dotdrift-specific path. On every Arch apply
EnsureInstalled hashes the installed plugin (`metadata.lua`,
`mise.plugin.toml`, `hooks/*` over fixed path:content pairs) and compares it to
the embedded plugin's SHA-256; it recopies only on mismatch, a missing plugin,
or when the target is a symlink (a stale shape from an older dotdrift). The
common up-to-date path performs no writes — it just hashes and runs. Because
the plugin is a normal installed plugin in mise's registry, there is no
`[bootstrap.plugins]` declaration and the apply runs only
`mise bootstrap --only packages` (no `plugins` phase), so the former
`Plugin paru already installed` / `Use --force` chatter is gone.
