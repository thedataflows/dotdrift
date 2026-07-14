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
