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
│   └── modules/<id>/...        # host overlays
└── users/<username>/
    └── modules/<id>/...        # user overlays (highest precedence)
```

See `examples/simple/` for a minimal profile.

> Note: `dotdrift apply` stores resume state and generated mise config under the XDG state directory (`$XDG_STATE_HOME/dotdrift/`, defaulting to `~/.local/state/dotdrift/`) so the profile directory is never polluted with runtime state. `dotdrift onboard` does the same (`.../profiles/<hash>/onboard/mise.toml`); pass `--yes` to answer mise prompts non-interactively.

> sudo warning: `dotdrift` resolves the username from the OS account, not `$USER`. Running `sudo dotdrift apply` selects **root's** overlays and writes into root's `HOME`. To manage your own dotfiles, run `dotdrift` as your normal user; use `sudo` only if you intentionally maintain a `users/root/` overlay.

## Commands

| Command | Purpose |
|---------|---------|
| `dotdrift init [path|git-url]` | Create a new profile (git-initialized) or clone a profile repo |
| `dotdrift detect` | Print host/user/os/distro/gpu/backend facts |
| `dotdrift modules` | List selected and skipped modules |
| `dotdrift plan` | Print the effective plan without side effects |
| `dotdrift apply [--yes]` | Run the full pipeline and resume from state |
| `dotdrift status` | Show resume cursor, selection, and last error |
| `dotdrift onboard <path>...` | Copy live paths into a module and apply |

## Testing

```bash
go test ./...
```

Unit tests run offline. Integration tests against real tools can be added with `//go:build integration`.

---

## License

[MIT License](LICENSE)
