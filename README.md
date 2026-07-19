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

## Testing

```bash
go test ./...
```

Unit tests run offline. Integration tests against real tools can be added with `//go:build integration`.

### Integration tests

```bash
./tests/e2e/run.sh
```

Builds and runs a Docker end-to-end suite for the debian family (`debian:bookworm-slim` and `ubuntu:24.04`; requires Docker and network access). Each container builds dotdrift from this repo, onboards a live file with a real `mise.run` bootstrap, and runs `dotdrift apply` against a fixture profile — then asserts a real `apt-get install curl` (verified via `dpkg`), dotfile symlinking, pre/post hooks executed as mise tasks, resume no-op on a second apply, complete state on disk, and no runtime pollution inside the profile. Runs on push to `main` via `.github/workflows/e2e.yml`; the offline `go test ./...` gate is unchanged.

---

## License

[MIT License](LICENSE)
