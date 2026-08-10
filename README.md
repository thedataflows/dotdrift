# dotdrift

A CLI tool for managing Linux configuration through git-backed profiles.

## Principles

- **Presence = managed**: every `modules/<id>/module.toml` is selected; no enable list.
- **Apply always resumes**: `dotdrift apply` continues from the first incomplete step.
- **Mise owns files**: symlink, copy, and template operations are performed by [mise](https://mise.jdx.dev).
- **Selection precedence**: module → host → user; user wins, `disable` is unioned across layers.

## Installation

### From a release (recommended)

Run the installer with `curl | bash`. It resolves the **latest** release, verifies the SHA-256 checksum, and installs the binary to `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/thedataflows/dotdrift/main/install.sh | bash
```

Pin a specific release, or install somewhere else:

```bash
curl -fsSL https://raw.githubusercontent.com/thedataflows/dotdrift/main/install.sh | bash -s v0.4.0
curl -fsSL https://raw.githubusercontent.com/thedataflows/dotdrift/main/install.sh | DOTDRIFT_BINDIR=/usr/local/bin bash
```

From a clone, run it directly: `./install.sh [version]`. It needs `curl`, `tar`, and `sha256sum`, and supports Linux on `amd64`/`arm64`. Each release ships prebuilt binaries with an SHA-256 checksum.

### From source

With [Go](https://go.dev) 1.26 or newer:

```bash
go install github.com/thedataflows/dotdrift@latest
```

### Runtime dependency

`dotdrift` requires [mise](https://mise.jdx.dev) ≥ 2026.8.2 on `PATH` (needed for `mise bootstrap` declarative convergence). When missing or too old, it installs a user-local copy via <https://mise.run>.

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

Each layer's `dotdrift.toml` may carry a `[modules] disable` list. Disables are unioned across the base, host, and user layers.

## Hooks

A module may declare shell commands to run around the apply pipeline:

```toml
# modules/<id>/module.toml
[hooks]
pre = ["echo about to apply"]
post = ["systemctl --user daemon-reload"]
```

- **When they run** — `pre` runs as the `hooks-pre` step, **before** packages are installed; `post` runs as `hooks-post`, **after** dotfiles.
- **How they run** — commands execute as [mise tasks](https://mise.jdx.dev) from the profile root, with `DOTDRIFT_PROFILE`, `DOTDRIFT_HOSTNAME`, `DOTDRIFT_USERNAME`, `DOTDRIFT_OS`, and `DOTDRIFT_BACKEND` in the environment.
- **Interactive commands** — when `dotdrift apply` has a terminal on stdin, hook tasks are generated with [`interactive = true`](https://mise.jdx.dev/tasks/task-configuration.html#interactive), so an interactive command in a hook (e.g. `sudo`) connects to the terminal and can disable echo on its password prompt instead of echoing it. With no terminal (piped/CI), hooks run normally.
- **How they merge** — unlike other sections, hooks are **appended** across layers (base → host → user) and modules, in selection order.
- **Failure and resume** — a failing hook fails its step, and resume re-runs it. Write `post` hooks to be idempotent.
- **Visibility** — hooks are listed in `dotdrift plan`. Skip them with `dotdrift apply --no-hooks` or `DOTDRIFT_NO_HOOKS=1`.

## Scope

A module applies to the home directory by default. Set `scope = "system"` in its `module.toml` to manage system dotfiles (e.g. targets under `/etc`):

```toml
# modules/<id>/module.toml
scope = "system"

[dotfiles]
"/etc/demo.conf" = { source = "demo.conf", mode = "copy" }
```

A single `dotdrift apply` covers both scopes. User dotfiles apply as usual; system dotfiles apply in a `dotfiles-system` step via `sudo`.

- One password prompt per apply, thanks to sudo's timestamp cache — none at all when already running as root.
- System-scope entries are marked `[system]` in `dotdrift plan`.
- Packages self-elevate via the distro backend, and hooks carry their own inline privilege, so neither needs scope machinery.

## Generate

`dotdrift generate` renders a derived module from declarations in `module.toml`:

- **`generate mounts`** — writes a system-scoped module holding systemd `.mount` units (plus `.service`/`.timer` when `--startat` is set).
- **`generate smb`** — writes a module holding `shares.conf` and a one-time `smb.conf` seed.

Mounts and shares are declared as `[mounts.<name>]` and `[smb]` / `[smb.shares.<name>]` tables in the module's `module.toml`, merged whole-entry by name across layers. The rendered files are derived artifacts.

- **Interactive vs strict mode** — on a terminal with no input flags, an interactive wizard runs (byte-identical result to the flag mode); with any input flag, strict CLI mode applies.
- **Layer selection** — `--layer base|host|user` picks where the module lands. A host-layer-only module needs no base stub.
- **Batching** — one CLI run writes one mount, since the `[mounts]` section is replaced wholesale. Use the wizard's "add another mount?" loop, or one `--module` per mount, for several mounts. `--share` is repeatable.
- **Activation** — the regular apply pipeline places the files via mise and activates them in the conditional `mounts` / `smb` steps (unit enablement, timers, samba group/users/service).

See `docs/product/cli-surface.md` for the full flag reference, and `docs/product/migrate-pimp-my-cachyos.md` for a worked migration. See `examples/simple/` for a minimal single-module profile, and `examples/profile/` for a multi-layer example with host and user overlays.

> **Note:** `dotdrift apply` stores the resume cursor while applying (and generated mise config) under the XDG state directory (`$XDG_STATE_HOME/dotdrift/`, defaulting to `~/.local/state/dotdrift/`), so the profile directory is never polluted with runtime state. The cursor file is deleted on completion, so a successful apply leaves no state file. `dotdrift onboard` does the same (`.../profiles/<hash>/onboard/mise.toml`); pass `--yes` to answer mise prompts non-interactively.

> **sudo warning:** `dotdrift` resolves the username from the OS account, not `$USER`. Running `sudo dotdrift apply` selects **root's** overlays and writes into root's `HOME`. To manage your own dotfiles, run `dotdrift` as your normal user; use `sudo` only if you intentionally maintain a `users/root/` overlay.

## Commands

| Command | Purpose |
|---------|---------|
| `dotdrift init [path&#124;git-url]` | Create a new profile (git-initialized) or clone a profile repo. |
| `dotdrift detect` | Print host/user/os/kernel/distro/gpu/backend facts. |
| `dotdrift modules [modules...]` | List selected and skipped modules (optionally limited to the listed modules). |
| `dotdrift plan [--json] [modules...]` | Print the effective plan without side effects (`--json` for machine-readable output; optionally limited to the listed modules). |
| `dotdrift apply [--yes] [--no-hooks] [--verbose] [modules...]` | Run the full pipeline and resume from the last successful step (the cursor file is deleted on completion; optionally limited to the listed modules). |
| `dotdrift status [-v] [-j N]` | Show drift between the profile and the live system (packages, tools, dotfiles, mounts, smb), plus the resume cursor. Probes run concurrently (`-j N` workers; default: CPUs); `-v` streams per-probe progress (`checking <section>: <item>`) to stderr. Exits 0 even when drift is found. |
| `dotdrift onboard [--verbose] <path>...` | Copy live paths into a module and apply; re-running updates the module (refresh files, merge `module.toml`). |
| `dotdrift generate mounts&#124;smb` | Generate a mounts module (systemd units) or smb module (samba shares) into a profile layer; interactive wizard on a terminal, strict flag mode otherwise. |

`-v` / `--verbose` (also `DD_VERBOSE=1`) streams package manager and mise output live on `apply` and `onboard`, echoing each command line `set -x`-style to stderr immediately before it runs — e.g. `+ paru -S --needed --noconfirm jq`; without it child-process output is captured and only surfaced in errors.

```bash
# Generate an NFS mount module with a nightly timer, then two samba shares
dotdrift generate mounts --name syn01 --source nas:/volume1/syn01 \
  --destination /mnt/synology/syn01 --type nfs --startat "*-*-* 18:05:00"
dotdrift generate smb --share media=/srv/media --share data=/mnt/data --no-avahi
```

## Module filter

`modules`, `plan`, and `apply` accept optional positional module ids that limit the command's scope to those modules. Ids are space- or comma-separated (both forms mix freely):

```bash
dotdrift apply vim git      # only vim and git
dotdrift apply vim,git      # same
dotdrift plan shell --json  # plan for shell only
```

With no ids the behavior is unchanged.

- **Unknown id** — a loud error naming the unknown ids and listing the valid module ids.
- **Selected but skipped** — naming a module that exists but is not selected (disabled via `[modules] disable` or excluded by a `when` filter) is also an error, naming it and its skip reason. The filter never resurrects skipped modules.

> **Resume caveat:** the resume cursor is a bare step name. A filtered apply that resumes from another run's cursor cannot detect the scope change, so it may skip steps the filtered run would have executed — delete the state file (path shown by `dotdrift status`) to force a full run.

## Testing

```bash
go test ./...
```

Unit tests run offline. Integration tests against real tools can be added with `//go:build integration`.

### Integration tests

```bash
./tests/e2e/run.sh
```

Builds and runs a Docker end-to-end suite across three distros — the debian family (`debian:bookworm-slim`, `ubuntu:24.04`) and CachyOS (`cachyos/cachyos`). It requires Docker and network access.

Each container builds dotdrift from this repo, onboards a live file with a real `mise.run` bootstrap, and runs `dotdrift apply` against a fixture profile. The suite then asserts:

- a real package install (`apt` on the debian family, `pacman`/`paru` on CachyOS) of a leaf package (`jq`);
- dotfile symlinking;
- `pre`/`post` hooks executed as mise tasks;
- a second apply runs the full pipeline again;
- no state file left on disk; and
- no runtime pollution inside the profile.

The CachyOS image is the first coverage of dotdrift's Arch backend (`paru -S` install + `pacman -Q` idempotency); it ships `paru` from the CachyOS binary repo, so no AUR build is needed. The suite runs on push to `main` via `.github/workflows/e2e.yml`; the offline `go test ./...` gate is unchanged.

---

## License

[MIT License](LICENSE)
