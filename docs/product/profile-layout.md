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
kernel = ">= 7.1"

[packages]
present = ["neovim", "ripgrep"]
absent = ["nano"]

[tools]
node = "20"
python = "3.12"

[dotfiles]
# Whole-file entries: source + mode.
"~/.bashrc" = { source = ".bashrc", mode = "symlink" }
"~/.config/nvim" = { source = "nvim", mode = "symlink-each" }
"~/.config/app/config.toml" = { source = "config.toml", mode = "copy" }
# Edit entries: partial edits keyed by "<file-path>/<edit-id>" (any scope).
"~/.zshrc/activate" = { block = 'eval "$(mise activate zsh)"' }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"~/.zshrc/snippet" = { source = "snippets/zsh-snippet.sh", mode = "edit" }
"~/.gitconfig/id" = { source = "snippets/git.tmpl", template = "tera" }

[hooks]
# A plain string is a required hook; an inline table { command, optional }
# flags a single hook non-fatal without leaving the array or losing its order.
pre = ["echo about to apply", { command = "echo warm best-effort cache", optional = true }]
post = ["echo apply finished"]

[mounts.data]
source = "UUID=abcd-1234"
destination = "/mnt/data"
type = "ext4"
options = ["noatime", "nofail"]
startat = "18:00"
state = "enabled"

[smb]
group = "smb"
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
  `kernel` is one `"<op> <version>"` constraint (`<`, `<=`, `>`, `>=`, `==`,
  `!=`), compared numerically per dotted segment against the running kernel
  release (`7.10 > 7.1`, missing segments are zero, distro suffixes like
  `-arch1-1` are ignored) — the same comparison as the generate registry's
  `recommended_if`, minus the redundant `kernel` keyword. An `-rcN` suffix
  marks a pre-release: `7.1-rc5` sorts after `7.0` but before `7.1` (the rc
  iteration is ignored). A malformed
  constraint is a load-time error naming the module; an empty kernel fact
  (detection failed) never matches a non-empty constraint. Use it for
  kernel-gated packages, e.g. one module with `kernel = "< 7.2"` installing
  `ntfs-3g` and another with `kernel = ">= 7.2"` installing `ntfsprogs-plus`.
- `scope` is module-level: `"user"` (the default when omitted) or `"system"`.
  It decides how the module's dotfiles are applied — user-scope entries are
  applied as the invoking user, system-scope entries are applied with root
  privileges during `dotdrift apply`. Dotfile targets under `/etc` (or other
  root-owned paths) belong in a `scope = "system"` module. Any other value is
  a resolve-time error naming the module and the value.
- `packages.absent` cancels a `present` entry from a lower layer.
- `dotfiles` entries come in two kinds, distinguished by their fields:
  - **Whole-file entries** take over a target path entirely. The key is a
    target path (absolute or `~/...`); the value is a table with:
    - `source`: relative path inside the module directory.
    - `mode`: `symlink`, `symlink-each`, `copy`, or `template` — exactly mise's
      mode vocabulary, passed through to the generated `mise.toml` unchanged.
      Any other value (including an omitted mode) is a resolve-time error naming
      the module and the mode — mise silently ignores entries with an
      unrecognized mode, so dotdrift fails loudly instead.
      `symlink-each` requires the source to be a directory; each file inside it
      is symlinked individually into the target directory.
  - **Edit entries** are partial edits to a file something else owns. The key
    is `<file-path>/<edit-id>` (the last slash splits file path from edit id);
    the value uses one of four forms:
    - `line = "..."`: ensure an exact line exists in the file.
    - `block = "..."` (optionally `comment = "#"`): wrap the block in mise
      marker delimiters (`>>> mise:<edit-id> >>>` / `<<< mise:<edit-id> <<<`),
      prefixed with the comment character. Re-apply updates the block in place.
    - `source = "..."` + `mode = "edit"`: read the source file's raw contents
      and use them as a block (dotdrift convenience — mise has no raw-source
      block form; `source` alone is a whole-file entry). The source is
      resolved across layers at plan time and the contents become the block.
      `comment` is supported, same as inline `block`. Avoids multi-line inline
      content in `module.toml`.
    - `source = "..."` + `template = "tera"`: render the source via the engine
      and insert it as a block (mise renders at apply time). `source` is
      resolved across layers like a whole-file source.
    Edit-entry rules (each violation is a resolve-time error naming the module
    and key): set at most one of `line`/`block`/`template`; `mode = "edit"`
    requires `source` and is exclusive with `line`/`block`/`template`; any
    other `mode` on an edit entry is an error; `comment` applies only to
    `block` and `mode = "edit"`; the key must be `<file-path>/<edit-id>` with a
    non-empty file path (not `~`, not `/`) and an edit id matching
    `[A-Za-z0-9._-]+`.
    Edit entries work at either scope, like whole-file entries. User-scope
    edits apply as the invoking user; system-scope edits (e.g. `/etc/hosts/...`)
    apply via elevated `mise dotfiles apply` (`sudo`) — the only mechanism for
    in-place edits, since `[bootstrap.files]` does file placement, not edits.
- Higher layers (user > host > module) override lower layers for the same
  dotfile key. For edit entries the key is the full `<file-path>/<edit-id>`,
  so a higher layer overrides only the same edit id on the same file; other
  edit ids on that file survive.
- Two selected modules claiming the same dotfile target is a resolve-time
  error naming the target and every claiming module (it would otherwise
  produce a duplicate-key `mise.toml` that mise refuses to parse). An edit
  entry on a file another module claims as a whole-file target is likewise a
  conflict — mise refuses to edit through a managed symlink, so dotdrift
  fails at resolve naming both modules and the file.
- `hooks.pre` / `hooks.post` are arrays of shell commands run as mise tasks during
  `dotdrift apply` (`hooks-pre` before packages, `hooks-post` after dotfiles).
  Unlike every other section, hooks merge by **append** across layers — base,
  then host, then user — and aggregate across modules in selection order;
  nothing is deduplicated or overridden. Each command runs as its own mise
  task in order; a non-zero exit fails the step and resume re-runs it. To mark
  a single hook non-fatal (its failure is logged at warn and the remaining
  hooks still run, so a flaky/best-effort hook cannot abort the apply) use the
  inline spelling `{ command = "...", optional = true }` — it keeps the compact
  array form and the hook's position (interleaving). The table-array spelling
  `[[hooks.pre]] command = "..." optional = true` is equivalent but verbose;
  bare strings stay all-required. `dotdrift plan` marks optional hooks
  (`(optional)` in text, `"optional": true` in JSON).
- `mounts` declares filesystem attachments as keyed tables `[mounts.<name>]`
  (ADR-0002). Mounts and smb require `scope = "system"` — their artifacts
  land in root-owned paths, so a user-scope module declaring either is a
  resolve-time error naming the module. Keys are mount names; values are tables with:
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
  - `group`: Linux group granted share access (`valid users = @<group>`;
    created at apply time). Defaults to `smb`.
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
