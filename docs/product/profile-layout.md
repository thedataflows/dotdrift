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
  The same holds per layer: `hosts/<hostname>/modules/<id>` and
  `users/<username>/modules/<id>` are selected by presence too, so a module
  existing only in a host or user layer (an overlay-only module) is managed
  like a base module. A host/user directory with the same name as a base
  module is that module's overlay, not a second module.
- `id` is the directory name unless overridden by `id` in `module.toml`.
- `app` defaults to `id` unless overridden.

## Validation

- `modules/` must exist. Loading a directory without it fails with
  `not a dotdrift profile: <path> missing modules/ directory` — a typo'd
  profile path is never silently treated as an empty profile.
- Module IDs must be unique across all scanned layers (`modules/`,
  `hosts/<hostname>/modules/`, `users/<username>/modules/`). Directories
  sharing a name across layers are overlays of one module (discovery
  precedence base → host → user: the representative path/config is the base
  layer's). Two different directory names resolving to the same `id` (via
  directory name or `id` override) fail with an error naming both module
  paths.
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
scope = "user"

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

[hooks]
pre = ["echo about to apply"]
post = ["echo apply finished"]

[mounts.data]
source = "UUID=abcd-1234"
destination = "/mnt/data"
type = "ext4"
options = ["noatime", "nofail"]
startat = "18:00"
state = "enabled"

[smb]
group = "WORKGROUP"
users = ["cri", "media"]
avahi = true

[smb.shares.media]
path = "/mnt/data/media"
comment = "Media library"
valid_users = "cri, media"
writable = true
public = false
```

- `when` filters are ANDed. An empty list means "any". `gpu` empty means any.
- `scope` is module-level: `"user"` (the default when omitted) or `"system"`.
  It decides how the module's dotfiles are applied — user-scope entries are
  applied as the invoking user, system-scope entries are applied with root
  privileges during `dotdrift apply`. Dotfile targets under `/etc` (or other
  root-owned paths) belong in a `scope = "system"` module. Any other value is
  a resolve-time error naming the module and the value.
- `packages.absent` cancels a `present` entry from a lower layer.
- `dotfiles` keys are target paths (absolute or `~/...`). Values are tables with:
  - `source`: relative path inside the module directory.
  - `mode`: `link`, `symlink-each`, `copy`, or `template`. Any other value
    (including an omitted mode) is a resolve-time error naming the module and
    the mode — mise silently ignores entries with an unrecognized mode, so
    dotdrift fails loudly instead.
  - `link` is dotdrift's vocabulary: it is translated to mise's `symlink`
    when the `mise.toml` is generated (real mise does not accept `link`).
    `copy`, `template`, and `symlink-each` pass through unchanged.
    `symlink-each` requires the source to be a directory; each file inside it
    is symlinked individually into the target directory.
- Higher layers (user > host > module) override lower layers for the same dotfile target.
- `hooks.pre` / `hooks.post` are arrays of shell commands run as mise tasks during
  `dotdrift apply` (`hooks-pre` before packages, `hooks-post` after dotfiles).
  Unlike every other section, hooks merge by **append** across layers — base,
  then host, then user — and aggregate across modules in selection order;
  nothing is deduplicated or overridden.
- `mounts` declares filesystem attachments as keyed tables `[mounts.<name>]`
  (ADR-0002). Keys are mount names; values are tables with:
  - `source`: what to attach (e.g. `UUID=...` for volumes, `//server/share`
    for network mounts). Required — empty is a resolve-time error naming the
    module and mount.
  - `destination`: where to attach it. Required — same error contract.
  - `type`: filesystem type (e.g. `ext4`, `btrfs`, `nfs`, `cifs`). Required,
    but **never validated against a registry** — any string resolves; the
    mount-type registry lives outside resolve and evolves independently.
  - `options`: list of mount options. Optional.
  - `startat` (one word, all lowercase): optional schedule at which the
    mount is started.
  - `state`: `enabled` or `disabled`; omitted means the apply default. Any
    other value is a resolve-time error naming the module, mount, and value.
  - Layers merge **whole-entry by name**: a higher layer's `[mounts.<name>]`
    fully replaces the lower layer's entry — no field-level merge.
- `smb` declares Samba server settings and shares:
  - `group`: workgroup name.
  - `users`: list of Samba users. Layers **replace** the list wholesale when
    set (non-empty); never appended — the deliberate contrast to hooks.
  - `avahi`: boolean advertising shares via Avahi/mDNS. When omitted the
    default is **true** (advertise); set `avahi = false` to disable. The
    omitted and explicit-false cases are distinguishable in the schema.
  - Scalar fields (`group`, `users`, `avahi`) merge by **replacement when
    the higher layer sets them**; an unset key in a higher layer leaves the
    lower layer's value in place.
  - `[smb.shares.<name>]` declares one share. `path` is required — empty is a
    resolve-time error naming the module and share. `comment`, `valid_users`
    (with the underscore spelling), `writable`, and `public` are optional.
    Shares merge **whole-entry by name** across layers, exactly like mounts.
    A share's path may coincide with a mount's destination, but shares and
    mounts are declared independently — no derivation exists between them.
