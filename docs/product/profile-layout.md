---
type: Specification
title: Profile layout
description: Git-backed profile directory structure and TOML schemas.
tags: [product, profile]
timestamp: 2026-07-14T00:00:00Z
---

# Layout

```
profile/
├── dotdrift.toml
├── modules/
│   └── <id>/
│       ├── module.toml
│       └── ... files used by dotfile entries
├── hosts/<hostname>/
│   ├── dotdrift.toml
│   └── modules/<id>/
│       ├── module.toml
│       └── ... overlay files
└── users/<username>/
    ├── dotdrift.toml
    └── modules/<id>/
        ├── module.toml
        └── ... overlay files
```

- `modules/<id>` is selected by presence if it has a valid `module.toml`.
- `id` is the directory name unless overridden by `id` in `module.toml`.
- `app` defaults to `id` unless overridden.

## Validation

- `modules/` must exist. Loading a directory without it fails with
  `not a dotdrift profile: <path> missing modules/ directory` — a typo'd
  profile path is never silently treated as an empty profile.
- Module IDs must be unique across `modules/`. Two modules resolving to the
  same `id` (via directory name or `id` override) fail with an error naming
  both module paths.
- Host/user overlays require a non-empty hostname/username. An empty value
  collapses the overlay path onto the parent directory (e.g.
  `hosts/dotdrift.toml`); if a file exists at that collapsed path, loading
  fails with `empty <hostname|username>: refusing to load collapsed overlay
  <path>`. If no file exists there, the overlay is treated as absent.
  Rationale: erroring unconditionally would forbid partial facts used purely
  for `when` filtering; erroring only on a real collapsed file keeps the
  silent-merge bug loud without breaking legitimate partial-fact loads.
- When no modules are selected, `dotdrift plan` prints
  `warning: no modules selected` before the plan body.

# `dotdrift.toml`

```toml
[modules]
disable = ["id1", "id2"]
```

- `disable` is unioned across base, host, and user layers (any disable sticks).

# `module.toml`

```toml
id = "optional-id"
app = "optional-app"

[when]
hosts = ["myhost"]
users = ["cri"]
os = ["arch", "cachyos"]
gpu = "nvidia"

[packages]
present = ["neovim", "ripgrep"]
absent = ["nano"]

[tools]
node = "20"
python = "3.12"

[dotfiles]
"~/.bashrc" = { source = ".bashrc", mode = "link" }
"~/.config/nvim" = { source = "nvim", mode = "symlink-each" }
"~/.config/app/config.toml" = { source = "config.toml", mode = "copy" }
```

- `when` filters are ANDed. An empty list means "any". `gpu` empty means any.
- `packages.absent` cancels a `present` entry from a lower layer.
- `dotfiles` keys are target paths (absolute or `~/...`). Values are tables with:
  - `source`: relative path inside the module directory.
  - `mode`: `link`, `symlink-each`, `copy`, or `template`.
- Higher layers (user > host > module) override lower layers for the same dotfile target.
