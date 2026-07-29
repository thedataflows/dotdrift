# dotdrift

A CLI tool for managing Linux configuration through git-backed profiles.

## Principles

- **Presence = managed**: every `modules/<id>/module.toml` is selected; no enable list.
- **Apply always resumes**: `dotdrift apply` continues from the first incomplete step.
- **Mise owns files**: symlink, copy, and template operations are performed by [mise](https://mise.jdx.dev).
- **Selection precedence**: module → host → user; user wins, `disable` is unioned across layers.

## Installation

```bash
go install github.com/thedataflows/dotdrift@latest
```

`dotdrift` requires [mise](https://mise.jdx.dev) on PATH. It will install a user-local copy via <https://mise.run> when missing or too old.

## Quick start

```bash
# Create a profile
dotdrift init ./my-profile
cd ./my-profile

# Onboard an existing config path into a module
dotdrift onboard ~/.bashrc

# See what would change
dotdrift plan

# Apply the profile (always resumes)
dotdrift apply --yes

# Show current state
dotdrift status
```

## Profile layout

```text
profile/
├── dotdrift.toml
├── modules/
│   └── <id>/
│       ├── module.toml
│       └── home/...          # files referenced by dotfile entries
├── hosts/<hostname>/
│   ├── dotdrift.toml           # host layer: disable list unioned
│   └── modules/<id>/...        # host overlays
└── users/<username>/
    ├── dotdrift.toml           # user layer: disable list unioned
    └── modules/<id>/...        # user overlays (highest precedence)
```

The `dotdrift.toml` in each layer may carry a `[modules]` `disable` list; disables are unioned across base, host, and user layers.

## Hooks

A module may declare shell commands to run around the apply pipeline:

```toml
# modules/<id>/module.toml
[hooks]
pre = ["echo about to apply"]
post = ["systemctl --user daemon-reload"]
```

`pre` commands run as the `hooks-pre` step **before** packages are installed; `post` commands run as the `hooks-post` step **after** dotfiles. Commands execute as [mise tasks](https://mise.jdx.dev) from the profile root with `DOTDRIFT_PROFILE`, `DOTDRIFT_HOSTNAME`, `DOTDRIFT_USERNAME`, `DOTDRIFT_OS`, and `DOTDRIFT_BACKEND` in the environment. Unlike other sections, hooks merge by **append** across layers (base → host → user) and modules, in selection order. A failing hook fails its step and resume re-runs it, so write post-hooks to be idempotent. Hooks are listed in `dotdrift plan`; skip them with `dotdrift apply --no-hooks` or `DOTDRIFT_NO_HOOKS=1`.

## Scope

A module applies to the home directory by default. Set `scope = "system"` in
its `module.toml` to manage system dotfiles (e.g. targets under `/etc`):

```toml
# modules/<id>/module.toml
scope = "system"

[dotfiles]
"/etc/demo.conf" = { source = "demo.conf", mode = "copy" }
```

One `dotdrift apply` covers both scopes: user dotfiles apply as usual, and
system dotfiles apply in a `dotfiles-system` step via `sudo` (one password
prompt per apply, thanks to sudo's timestamp cache; no sudo at all when
already running as root). System-scope entries are marked `[system]` in
`dotdrift plan`. Packages self-elevate via the distro backend and hooks carry
their own inline privilege, so neither needs scope machinery.

## Generate

`dotdrift generate mounts` writes a system-scoped module holding systemd
`.mount` units (plus `.service`/`.timer` for `--startat`), and
`dotdrift generate smb` writes one holding `shares.conf` and a one-time
`smb.conf` seed. Mounts and shares are declared as `[mounts.<name>]` and
`[smb]`/`[smb.shares.<name>]` tables in the module's `module.toml`
(merged whole-entry by name across layers); the rendered files are
derived artifacts. On a terminal with no input flags an interactive
wizard runs (same byte-identical result as the flag mode); with any
input flag the strict CLI mode applies. `--layer base|host|user` picks
where the module lands — a host-layer-only module needs no base stub.
One CLI run writes one mount (the `[mounts]` section is replaced
wholesale, so use the wizard's "add another mount?" loop or one
`--module` per mount for several); `--share` is repeatable. The regular
apply pipeline then places the files via mise and activates them in the
conditional `mounts`/`smb` steps (unit enablement, timers, samba
group/users/service). See `docs/product/cli-surface.md` for the full
flag reference and `docs/product/migrate-pimp-my-cachyos.md` for a
worked migration.

See `examples/simple/` for a minimal single-module profile, and `examples/profile/` for a multi-layer example with host and user overlays.

> Note: `dotdrift apply` stores resume state and generated mise config under the XDG state directory (`$XDG_STATE_HOME/dotdrift/`, defaulting to `~/.local/state/dotdrift/`) so the profile directory is never polluted with runtime state. `dotdrift onboard` does the same (`.../profiles/<hash>/onboard/mise.toml`); pass `--yes` to answer mise prompts non-interactively.

> sudo warning: `dotdrift` resolves the username from the OS account, not `$USER`. Running `sudo dotdrift apply` selects **root's** overlays and writes into root's `HOME`. To manage your own dotfiles, run `dotdrift` as your normal user; use `sudo` only if you intentionally maintain a `users/root/` overlay.

## Commands

| Command | Purpose |
|---------|---------|
| `dotdrift init [path|git-url]` | Create a new profile (git-initialized) or clone a profile repo |
| `dotdrift detect` | Print host/user/os/distro/gpu/backend facts |
| `dotdrift modules` | List selected and skipped modules |
| `dotdrift plan [--json]` | Print the effective plan without side effects (`--json` for machine-readable output) |
| `dotdrift apply [--yes] [--no-hooks]` | Run the full pipeline and resume from state |
| `dotdrift status` | Show resume cursor, selection, and last error |
| `dotdrift onboard <path>...` | Copy live paths into a module and apply (`--force` replaces a conflicting module copy with the live file) |
| `dotdrift generate mounts\|smb` | Generate a mounts module (systemd units) or smb module (samba shares) into a profile layer; interactive wizard on a terminal, strict flag mode otherwise |

```bash
# Generate an NFS mount module with a nightly timer, then two samba shares
dotdrift generate mounts --name syn01 --source nas:/volume1/syn01 \
  --destination /mnt/synology/syn01 --type nfs --startat "*-*-* 18:05:00"
dotdrift generate smb --share media=/srv/media --share data=/mnt/data --no-avahi
```

## Testing

```bash
go test ./...
```

Unit tests run offline. Integration tests against real tools can be added with `//go:build integration`.

### Integration tests

```bash
./tests/e2e/run.sh
```

Builds and runs a Docker end-to-end suite across three distros — the debian family (`debian:bookworm-slim`, `ubuntu:24.04`) and CachyOS (`cachyos/cachyos`); requires Docker and network access. Each container builds dotdrift from this repo, onboards a live file with a real `mise.run` bootstrap, and runs `dotdrift apply` against a fixture profile — then asserts a real package install (`apt` on the debian family, `pacman`/`paru` on CachyOS) of a leaf package (`jq`), dotfile symlinking, pre/post hooks executed as mise tasks, resume no-op on a second apply, complete state on disk, and no runtime pollution inside the profile. The CachyOS image is the first coverage of dotdrift's Arch backend (`paru -S` install + `pacman -Q` idempotency); it ships `paru` from the CachyOS binary repo, so no AUR build is needed. Runs on push to `main` via `.github/workflows/e2e.yml`; the offline `go test ./...` gate is unchanged.

---

## License

[MIT License](LICENSE)
