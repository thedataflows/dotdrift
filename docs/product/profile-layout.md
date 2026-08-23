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

- `module.toml` is validated **strictly against the schema below**: any key
  outside it (a typo like `preset` under `[packages]`, a misplaced table) is
  a load-time error naming the file, line, and key —
  `<path>/module.toml:<line>: unknown key "<key>" [in <table>]`. Values of
  the wrong shape (TOML parse errors, or a type mismatch like
  `disabled = "a"` on a boolean key) fail with the same reporting bar —
  `<path>/module.toml:<line>:<col>: <message>` plus the offending source
  line and a caret under the bad token; for a known scalar key the message
  also names the expected type (`"disabled" expects a boolean: true or
  false`). Every
  module.toml read (all layers, resolve, onboard, generate, TUI) applies the
  same check, so an invalid file fails every command immediately instead of
  silently decoding to zero values. Structured hook tables
  (`[[hooks.pre]]`/`[[hooks.post]]`) accept only `command` and `optional`.
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

[colors]
# Optional per-role overrides of the colored-output palette. Values are raw
# ANSI SGR parameters ("31", "1;31", "38;5;208"), not escape sequences.
missing = "38;5;75"
orphan  = "94"
```

- `disable` is unioned across base, host, and user layers (any disable sticks).
- `colors` overrides the colored-output palette per role. Roles: `ok`
  (green, all-checks-passed / `no drift` / the clean `resume:` line),
  `missing` (**orange**, missing
  or removed items), `warn` (yellow, content/version differs, and the
  pending `resume:` line when a cursor exists), `error`
  (red, unknown / not-a-symlink), `orphan` (magenta, the status orphans
  section), `dim` (bright black, dimmed module/description text). Values
  are raw SGR parameter strings — digits and semicolons only, no ESC/CSI
  wrapper (`"31"`, not `"\033[31m"`); anything else is a load-time error
  naming the file and key. Overrides apply from any layer with
  higher-layer precedence per role, affect `status`/`modules`/`plan`/diff
  output, and never re-enable color under `NO_COLOR`/`--no-color`.

# `module.toml`

```toml
id = "optional-id"
app = "optional-app"
scope = "user"
description = "optional human-readable summary"
disabled = false

[when]
hosts = ["myhost"]
users = ["cri"]
os = ["arch", "cachyos"]
gpu = "nvidia"
kernel = ">= 7.1"
packages = ["ntfs-3g"]
tools = ["node"]
# Combinators (all optional, nestable, recursive):
not = { packages = ["legacy"] }
or = [{ gpu = "amd" }, { os = ["fedora"] }]
and = [{ users = ["ops"] }]

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

- `when` is the module's **conditional-loading expression** — a boolean
  combination of system facts. Omitted/empty `[when]` always selects. The
  full grammar, evaluation rules, validation contract, and worked examples
  are specified in [The `[when]` filter](#the-when-filter) below.
- `scope` is module-level: `"user"` (the default when omitted) or `"system"`.
  It decides how the module's dotfiles are applied — user-scope entries are
  applied as the invoking user, system-scope entries are applied with root
  privileges during `dotdrift apply`. Dotfile targets under `/etc` (or other
  root-owned paths) belong in a `scope = "system"` module. Any other value is
  a resolve-time error naming the module and the value.
- `description` is an optional human-readable summary of the module. When set,
  `dotdrift modules` shows it appended to the module's line
  (`<marker> <id>… — <description>`). It is display-only metadata, not a
  resolution input, and is read from the representative layer like `scope`
  (an overlay's `description` is ignored).
- `disabled` is an optional boolean (`false` when omitted). When `true`, the
  module is skipped with reason "disabled" — the same effect as listing it in
  `dotdrift.toml`'s `[modules] disable`. Disables are **unioned** across layers:
  setting `disabled = true` in a host or user overlay disables the base module;
  an overlay setting `disabled = false` does not un-disable (any disable sticks).
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
    apply via `mise dotfiles apply`, retried elevated (`sudo`) when the OS
    denies access — the same try/retry pattern as whole-file system entries.
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

## The `[when]` filter

`[when]` is the module's conditional-loading expression: a **boolean
expression over system facts**. A module whose expression holds is
selected; one whose expression fails is skipped with reason
`when filter` (visible in `dotdrift modules`, and naming it on the CLI is
an error — the filter never resurrects a skipped module). An omitted or
empty `[when]` always selects. The expression is evaluated against the
facts detected at load time (hostname, username, os, kernel release,
gpu) plus lazily probed installed-state facts (see
[Installed-state probing](#installed-state-probing-packages--tools))).

### Leaves

Within one `[when]` table the leaf fields AND together; an omitted leaf
is ignored (empty list = "any", empty scalar = "any").

| Key | TOML type | Matches | Notes |
|---|---|---|---|
| `hosts` | list of strings | detected hostname | case-sensitive exact match |
| `users` | list of strings | detected username | case-sensitive exact match |
| `os` | list of strings | detected os (`linux`, …) | any-of list |
| `gpu` | **single string** | detected gpu (`nvidia`/`amd`/`intel`/`unknown`) | not a list |
| `kernel` | **single string** | running kernel release, `"<op> <version>"` | see below |
| `packages` | list of strings | installed **system packages** | all must be installed; probed; **each entry is an exact name or an anchored regex** |
| `tools` | list of strings | installed **mise-managed tools** | all must be installed; probed; **each entry is an exact name or an anchored regex** |

`kernel` holds exactly one `"<op> <version>"` constraint (`<`, `<=`, `>`,
`>=`, `==`, `!=`), compared numerically per dotted segment against the
running kernel release (`7.10 > 7.1`, missing segments are zero, distro
suffixes like `-arch1-1` are ignored) — the same comparison as the
generate registry's `recommended_if`. An `-rcN` suffix marks a
pre-release: `7.1-rc5` sorts after `7.0` but before `7.1` (the rc
iteration is ignored). An empty kernel fact (detection failed) never
matches a non-empty constraint; a malformed constraint is a load-time
error (see [Validation](#validation)).

`packages`/`tools` entries are matched **exact-first**: an installed
name exactly equal to the entry satisfies it (found by lookup, never
re-interpreted as a pattern). An entry containing regex metacharacters
(`. + * ? ( ) | [ ] { } ^ $ \`) that has no exact hit is matched as an
**anchored full-name regex** (`^(?:entry)$`) against the installed set —
`packages = ["apollo.*"]` matches both `apollo` and `apollo-cuda-git`
but not `xapollo`; `apollo.+` does not match bare `apollo`.
Metachar-free entries keep pure exact-match semantics. Every entry must
be a valid regex: a syntax error is a load-time error (see
[Validation](#validation)); write regex-special literals escaped —
`"g\\+\\+"` matches the package `g++`. Names are matched as written — no
`aur/`-marker or `manager:`-prefix normalization; write the name the
same way on both sides or the filter will not match. A name that is
absent or indeterminable fails its leaf (fail-open) but is never a
load-time error for a well-formed entry.

### Combinators

Three combinators build larger expressions. Each sub-expression has
exactly the shape of `[when]` again, so nesting is unbounded:

```toml
[when]
# leaves (ANDed) ...
or  = [ <when>, ... ]   # ANY-OF: at least one element must match
and = [ <when>, ... ]   # ALL-OF: every element must match (grouping)
not = { <when> }        # NEGATION: the sub-expression must NOT match
```

A node matches when, all together:

1. every leaf set on the node matches (plain AND),
2. every `and` element matches,
3. at least one `or` element matches — when the list is non-empty, and
4. the `not` sub-expression does **not** match.

`not` over a node with several leaves negates their **conjunction**
(De Morgan: `not = { gpu = "nvidia", kernel = "< 7" }` =
not-(gpu∧kernel) = not-gpu OR not-kernel). Combinators compose freely:
`not` of an `or`, `or` of `and`-groups, `and` of `not`s — any depth.

Both TOML spellings decode identically: inline tables
(`not = { ... }`) and dotted tables / arrays of tables (`[when.not]`,
`[[when.or]]`).

### Examples

```toml
# Kernel gate AND absence: "kernel >= 7 AND somepackage NOT installed"
[when]
kernel = ">= 7"
not = { packages = ["somepackage"] }

# Regex entries: matches apollo OR apollo-cuda-git (any apollo-* package):
[when]
packages = ["apollo.*"]

# Any-of over hardware or installed state:
[when]
or = [{ gpu = "nvidia" }, { packages = ["nvidia-driver"] }]

# Grouping: (arch or cachyos) and (nvidia or kernel >= 7):
[when]
and = [
  { or = [{ os = ["arch"] }, { os = ["cachyos"] }] },
  { or = [{ gpu = "nvidia" }, { kernel = ">= 7" }] },
]

# De Morgan: "neither nvidia GPU nor kernel < 7" (NOT of an or):
[when]
not = { or = [{ gpu = "nvidia" }, { kernel = "< 7" }] }

# Dotted spelling, identical to the inline forms above:
[when]
kernel = ">= 7"

[when.not]
packages = ["somepackage"]

[[when.or]]
gpu = "nvidia"

[[when.or]]
os = ["fedora"]
```

A complete, loadable example ships as
[`examples/simple/modules/conditional`](../../examples/simple/modules/conditional/module.toml).

### Validation

The expression is validated recursively at load, and a violation is a
**load-time error naming the module** — never a silent always-select or
never-select footgun. (An invalid `module.toml` fails every command, per
the strict-schema rule above.)

| Violation | Why it errors |
|---|---|
| malformed `kernel` constraint (any depth) | a typo must fail loudly, not silently never-match |
| invalid regex in a `packages`/`tools` entry (any depth) | a pattern that cannot compile would silently never-match |
| `not = {}` (empty table) | negates nothing — meaningless |
| `or = []` / `and = []` (explicit empty list) | `or` can never match; `and` carries no meaning |
| empty `or`/`and` element (e.g. `or = [{}]`) | the empty element would vacuously match, silently making the whole `or` always-true |
| unknown key inside any `[when]` table | strict schema — typos never decode silently |

The top-level node itself may be empty (`[when]` alone = always select);
only combinator sub-expressions must be non-empty.

### Installed-state probing (`packages` / `tools`)

Installed status for `packages`/`tools` leaves is probed **lazily at
profile load**, never at detection time:

- `packages` — one query per distinct name via the detected package
  backend (`paru`/`apt`/`dnf`, from the distro fact). When any regex
  entry exists anywhere in the profile, this becomes **one installed-list
  query** (`pacman -Qq` / `dpkg-query -W -f ${Package}` /
  `rpm -qa --qf %{NAME}`) — a list answers plain entries too, so a mixed
  profile still costs one subprocess. If the list is unavailable
  (unknown backend, query failure), plain entries fall back to exact
  per-name probes and regex entries fail open.
- `tools` — one query per distinct name via `mise current <tool>`
  (presence only, the version is ignored; no mise binary on PATH ⇒
  nothing matches). Regex tool entries switch this to **one
  `mise ls --json`** call (its output is an object keyed by tool name),
  same list-vs-exact strategy and fallback as packages.
- Names are collected from **the whole expression tree** — leaves inside
  `or`/`and`/`not` probed like top-level leaves, partitioned into plain
  and regex entries — and deduplicated across all modules.
- A profile declaring no `when.packages`/`when.tools` anywhere probes
  nothing: existing profiles cost zero extra subprocesses.
- A name that is not installed, or whose status cannot be determined
  (unknown backend, package-manager failure, missing mise), simply fails
  its leaf — fail-open, same as an empty kernel fact. Selection depends
  on it; load never does.

### Referenced sources, orphans, and adoption

A `[dotfiles]` declaration's **reference set** decides which module files
are doing work. References come from the layer declarations themselves —
not from a resolved plan, which covers one machine's view — evaluated per
host/user **view** (a view is one host's or user's overlay combination
over base; entries merge whole-entry by precedence, user > host > base):

- a declaration whose source is a **directory** references that whole
  subtree **anchored to the declaring layer** (mise links/copies
  directory trees wholesale, so every nested file deploys; this covers
  `symlink-each` and whole-dir `symlink`/`copy` alike). An overlay
  holding a dir at the same rel-path wins deployment, but the declaring
  layer's tree stays the authored reference — extra overlay files remain
  orphans;
- a **file** source references the file each view resolves it to
  (user > host > base, first existing): a base copy shadowed on every
  host is dead content, a copy that still resolves for a host without an
  overlay copy deploys there and stays referenced.

Files inside a module's layer directory that no declaration references
(`module.toml` itself excluded) are **orphans**. `dotdrift status` scans
EVERY layer directory in the profile — `modules/*`,
`hosts/*/modules/*`, `users/*/modules/*` — regardless of the current
host, user, or when-filter selection: orphans are profile-content drift,
so a leftover on another machine's overlay shows from anywhere, grouped
under its layer-root heading.

`dotdrift onboard` **adopts** orphans of the module it materializes
(issue 0015): every invertible orphan — `home/<rel>` maps back to
`~/<rel>`, `system/<rel>` to `/<rel>` — becomes a `[dotfiles]` entry in
that module.toml with the run's `--mode` (default `symlink`). A
directory whose entire content is orphaned collapses to ONE whole-dir
entry, bounded by two rules (issue 0017): claims merge across ALL of the
module's layers (a base `symlink-each` over `~/.config/app` keeps a
stray overlay file from collapsing into an ancestor), and a dir unit
never claims a shared namespace root (`~`, `~/.config`, `~/.local`,
`~/.cache`, `/`, `/etc`, `/usr`, `/var`, `/opt`) — with nothing bounding
the chain, a deep orphan adopts its own subdirectory instead. FILE units
are blocked only by an exact target duplicate, so a stray file under
another entry's subtree is still adoptable under its own path. The live
counterpart, when present, is snapshotted over the stale module copy
first (onboard snapshots live state), keeping the forced takeover apply
lossless; module-root files with no derivable target (hook scripts,
notes) are never adopted.

Passing a path INSIDE a module layer directory (a module file itself) is
a **directed adoption**: onboard adds the `[dotfiles]` entry for it in
that layer's module.toml — the path names its corresponding level — with
no copy and no live-path mapping. Notices name the layer:
`would adopt: <target> (<source>) [base|hosts/<hostname>|users/<username>]` in `--dry-run`,
`adopted: ...` in a real run, and the adopted entries ride the same mise
apply as the onboarded paths.

### Backups (`apply --backup`)

Copy is the only dotfile mode whose apply overwrites destination content
(symlinks are recreated as links, edit entries are marker-scoped).
`dotdrift apply --backup` snapshots every existing copy-mode destination
into the profile before the pipeline runs (issue 0025): each target —
user or system scope, directories recursively, symlinks followed — is
copied into the module layer directory that declared the entry, under
`backups/<generation>/`, mirroring the absolute target path:

```
modules/easyeffects/backups/20260824-153000/home/cri/.config/easyeffects/db/easyeffectsrc
hosts/cri-pc/modules/demo/backups/20260824-153000/etc/demo.conf
```

One timestamp generation is shared by every module in the run, so a
generation is coherent across modules; restoring is a plain copy back
along the mirrored path. All existing targets are backed up, not only
differing ones; missing targets are skipped; an unreadable target aborts
the apply (the flag must not fail open). Nothing is written without the
flag, and nothing when the dotfiles section is deselected.

A `backups/` directory directly under a module layer root is dotdrift
runtime output, not profile content: the status orphan scan skips it, and
onboard never adopts from it (its paths do not invert to live targets).
Deeper `backups` directories are ordinary content. Generations are never
pruned automatically — clean them by hand, and consider gitignoring
`backups/` in the profile repository.
