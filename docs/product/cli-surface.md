---
type: Specification
title: CLI surface
description: Commands and minimal flags for dotdrift.
tags: [product, cli]
timestamp: 2026-07-28T00:00:00Z
---

# Commands

| Command | Behavior |
|---------|----------|
| `dotdrift init [path-or-git-url]` | Local path: create the profile and git-initialize it. Git URL: clone into a dir named from the URL minus any trailing `.git`, relative to the given path; the clone must be a dotdrift profile (`dotdrift.toml` present) or init errors |
| `dotdrift detect` | Print facts (host, user, os, kernel, distro, gpu, package backend) |
| `dotdrift modules [modules...]` | List modules as `selected <id>` / `skipped  <id> <reason>`, one per line. Selected lines are tagless by default; a system-scope module carries `[system]`, and a module whose `app` differs from its id carries `(app: <app>)` (no `modules ls` form; bare `dotdrift` prints help and exits with a usage error). Optional positional module ids (space or comma separated) limit the listing; selected-but-excluded modules show as skipped with reason `module filter` |
| `dotdrift plan [--json] [--deps [--deps-depth N]] [modules...]` | Print effective plan; side-effect-free, never touches mise. Optional positional module ids (space or comma separated) limit the resolved plan's scope. `--deps` renders a dependency tree for every package in `packages.install` (read-only package-manager queries: `paru -Si` / `apt-cache depends` / `dnf repoquery --requires --resolve`); `--deps-depth N` (default 1, requires `--deps`, must be >= 1) recurses deeper. Text shape: `  - pkg` then `    deps:` and one `    - dep` per child, recursing two spaces per level; a package whose query fails renders `- pkg (deps unknown)` (warn-and-proceed, the plan never fails on a deps query). `--json` adds a recursive `packages.deps[]` (`name`, nested `deps`, `unknown`) under `--deps` and omits the key otherwise. `--json` emits a single JSON object (`fingerprint`, `modules`, `packages.install`/`remove`, `tools`, `dotfiles[]` with target/source/mode/module/layer/scope, `hooks.pre`/`post`, `mounts[]` with module/name/source/destination/type/options/startat/state/layer/scope, `smb[]` with module plus the merged spec — group/users/avahi (`null` when unset)/shares) instead of the text rendering; the no-modules warning is suppressed so stdout stays parseable. System-scope dotfiles are marked `[system]` in the text rendering. The text rendering ends with conditional sections when the profile declares them: `mounts:` lists each entry as `<module>: <name> <type> <source> -> <destination> [layer][scope]` plus `startat=`/`state=` markers when set; `smb:` lists per module the effective group (unset → `smb`), users, avahi (unset → `true`), and each share as `name -> path`. Both sections are omitted entirely when their aggregates are empty |
| `dotdrift apply [--yes] [--no-hooks] [-v|--verbose] [modules...]` | Full pipeline; always resumes from the last successful step (the cursor file is deleted on completion, so a completed apply's next run starts from the beginning). Optional positional module ids (space or comma separated) limit the run's scope to the listed modules. Step order: `hooks-pre` → `packages` → `tools` → `dotfiles` → `dotfiles-system` → `mounts` → `smb` → `hooks-post`. `--no-hooks` (or `DOTDRIFT_NO_HOOKS=1`) skips the `hooks-pre`/`hooks-post` steps. `-v` / `--verbose` (also `DD_VERBOSE=1`) streams mise operations, package manager commands, and mounts/smb activation commands live to the terminal instead of capture-and-discard, echoing each command line `set -x`-style (`+ <argv>`) to stderr immediately before it runs; probes (`mise --version`, installed-package checks) always stay captured and unechoed. When the plan contains `scope = "system"` modules, a `dotfiles-system` step applies their dotfiles via `sudo` (skipped when already root); sudo's timestamp cache means at most one password prompt per apply. `mounts` (mkdir destinations, `systemctl daemon-reload`, enable/disable units + startat timers) and `smb` (group/user membership, avahi, testparm gate, service enablement, samba accounts) are conditional activation steps constructed only when the plan declares mounts/smb; they never write config files — placement is mise's job |
| `dotdrift status` | Read-only drift report plus the resume cursor. Resolves the plan and probes the live system — packages (installed?), tools (`mise current`), dotfiles (symlink target / copy content / template existence), mounts (systemd unit enabled+active + destination dir), smb (group/users/services/shares) — printing one `ok`/`drift`/`unknown` line per item in fixed section order (a section whose findings are all OK prints `ok: all N checks passed`; sections with no plan items are omitted), a summary (`no drift` or `drift: N item(s), K unknown`), and the resume cursor line (`resume: clean …` or `resume: last completed "<step>" …`). Exits 0 even when drift is found; only real errors (detect/load/resolve/corrupt state) return non-zero. Tools report `unknown` when no mise binary resolves. `--state` only overrides the cursor path |
| `dotdrift onboard [-v|--verbose] <path>...` (alias: `add`) | Create or update a module; mise apply immediately. Re-running onboard **updates**: existing files are refreshed from the live path (directories replaced wholesale so deletions propagate) and `module.toml` is merged — new entries/packages are added, existing ones and any unmanaged sections (scope, when, hooks, mounts, smb) are preserved. Writes a light `module.toml` (inline `[dotfiles]` tables, no `id`/`app`). `-v` / `--verbose` (also `DD_VERBOSE=1`) streams the mise install/dotfiles output live, echoing each command line `set -x`-style (`+ <argv>`) to stderr before it runs |
| `dotdrift generate mounts` | Generate a mounts module: systemd `.mount` units (plus `.service`/`.timer` for `--startat`), `module.toml` with `scope = "system"`, `[mounts]`, packages, and dotfile entries targeting `/usr/local/lib/systemd/system`. Idempotent: same invocation → byte-identical tree; stale generated units are garbage-collected |
| `dotdrift generate smb` | Generate an smb module: `shares.conf`, a one-time `smb.conf` seed (then user-owned), `module.toml` with `scope = "system"`, `[smb]`, packages (`samba`, plus `avahi` unless disabled), and dotfile entries targeting `/etc/samba` |
| `dotdrift paru installed\|install` | Internal: mise package-plugin backend for the `paru` manager (pacman + AUR). `installed <names>` checks `pacman -Q` and emits a tab-separated status protocol; `install [--dry-run] [--update] <names>` runs `paru -S --needed --noconfirm`. Invoked by the Lua plugin shim, not by end users |

# Module filter

`modules`, `plan`, and `apply` share an optional positional module filter
(`dotdrift apply vim git` ≡ `dotdrift apply vim,git`; comma and space forms
mix, duplicates collapse, whitespace is trimmed). Semantics:

- No ids → behavior is unchanged.
- Unknown id → error naming the unknown ids and listing the valid module ids;
  nothing runs.
- Id naming a module that exists but is not selected (`[modules] disable` or
  `when` filter) → error naming it and its existing skip reason. The filter
  never resurrects skipped modules.
- Otherwise the command's scope shrinks to the listed modules; excluded
  selected modules are reported as skipped with reason `module filter`.

Resume cursor caveat: the cursor is a bare step name (the last successful
pipeline step). A filtered apply that resumes from another run's cursor cannot
detect the scope change, so it may skip steps that the filtered run would have
executed — delete the state file (path shown by `dotdrift status`) to force a
full run.

# Generate

Both subcommands write a self-contained module at one profile layer via the
same engine (`internal/generate.WriteModule`); the regular apply pipeline
(packages → dotfiles → mounts/smb steps) then activates what was generated.

## Shared flags

| Flag | Default | Notes |
|------|---------|-------|
| `--profile PATH` | `.` | Profile root |
| `--layer base\|host\|user` | `base` | Target layer: `modules/`, `hosts/<hostname>/modules/`, `users/<username>/modules/` |
| `--module ID` | `mounts` / `smb` | Module directory name |
| `--hostname H` | detected | Required resolution for `--layer host` (detected when omitted) |
| `--username U` | detected | Required resolution for `--layer user` (detected when omitted) |
| `--tui` / `--no-tui` | auto | `--tui` forces the interactive wizard (terminal required), even with input flags — they pre-fill the wizard; `--no-tui` forces CLI mode |

## `generate mounts` input flags

| Flag | Default | Notes |
|------|---------|-------|
| `--name NAME` | — | Mount name (`[mounts.<name>]`); required in CLI mode |
| `--source SRC` | — | e.g. `UUID=<uuid>` or `server:/export`; required in CLI mode |
| `--destination DEST` | — | Mount point; the unit stem is `EscapePath(DEST)`; required in CLI mode |
| `--type TYPE` | — | Filesystem type; must exist in the generate registry; required in CLI mode |
| `--option OPT` | registry preset for `--type` | Repeatable; when omitted entirely the registry preset applies (bare `uid`/`gid` tokens expand to the invoking user's ids) |
| `--startat CAL` | none | OnCalendar expression; adds a `.service` + `.timer` pair |
| `--state enabled\|disabled` | `enabled` | Recorded in the mount spec |
| `--list-volumes` | off | Print the detected volume table (`UUID FSTYPE LABEL SIZE MOUNTPOINTS MANAGED`) and exit 0; `MANAGED` marks UUIDs already used as sources in the target module's `[mounts]` (absent module tolerated) |

## `generate smb` input flags

| Flag | Default | Notes |
|------|---------|-------|
| `--share name=path` | — | Repeatable; at least one required in CLI mode; both sides must be non-empty |
| `--group G` | `smb` | Samba group for `valid users = @<group>` |
| `--user U` | invoking user | Repeatable |
| `--avahi` / `--no-avahi` | on (unset) | No flag: `avahi` key left unset (default-on semantics); `--no-avahi` records an explicit `avahi = false` and drops the `avahi` package |
| `--writable` / `--no-writable` | on | `--readonly` is the shorthand for `--no-writable` (and wins when both are given) |
| `--readonly` | off | Sets `writable = false` on every share |
| `--public` | off | Guest access on every share |

## Mode selection (strict, both subcommands)

1. Any input flag present → **CLI mode**: the required input flags are
   validated loudly (the error names each missing flag), a
   `generate.Input` is assembled, `WriteModule` runs, and a summary
   (module dir + written files) is printed.
2. No input flags + terminal → the interactive **wizard** (see below).
3. No input flags + no terminal → an actionable error listing the flags
   CLI mode needs.
4. `--tui` without a terminal → loud error.

## The interactive wizard

The wizard (charmbracelet huh forms over a pure spec-builder state
machine, lipgloss chrome) owns everything after mode selection,
including `WriteModule`.

- **Tabs.** A lipgloss tab bar shows `mounts | smb` with the active
  flow highlighted; a top-level select (`mounts | smb | quit`) acts as
  the tab switcher. The invoking subcommand preselects its tab;
  switching runs the other flow with the same profile/layer values.
- **Pre-fill.** Any already-parsed input flags (`--tui` forces the
  wizard even with flags) become the wizard's defaults: the first
  mount's fields, or the smb group/users/avahi/shares.
- **Mounts steps.** (1) layer select + module id (+ hostname/username
  inputs shown only for the matching layer, defaulting to detected
  values); (2) kind select: volume | network; (3a) volume: a
  MultiSelect of lsblk-detected volumes with already-managed ones
  marked `[managed]` (on lsblk failure the error is shown and a
  manual-source fallback is offered), then per volume: name, type
  select with the kernel-recommended entry preselected and marked
  `(recommended)`, destination (default `/mnt/<label-or-uuid-short>`),
  options MultiSelect (registry preset pre-checked) plus free-form
  additions; (3b) network: source input validated as `host:/export`
  (nfs-style) or `//host/share` (cifs-style), type select, same
  options step; (4) optional `--startat` schedule via a confirm +
  OnCalendar input; (5) state select (enabled/disabled); (6) a review
  screen and keep-confirm per mount.
- **Multiple mounts, one write.** After a mount is kept, "add another
  mount?" loops back to the kind select. All accumulated mounts are
  written with ONE `WriteModule` call at the end (a mid-loop write
  would wipe earlier iterations, since `[mounts]` is replaced
  wholesale). Aborting (ctrl+c/esc) before the end writes nothing.
- **Smb steps.** Group (default `smb`), users (comma-separated, default
  the invoking user), avahi confirm (default yes — a "yes" keeps the
  key unset like the CLI default, a "no" records `avahi = false`), then
  a shares loop (name, path, comment, writable confirm default yes,
  public confirm default no, "add a share?"), a review, and one
  `WriteModule` call.
- **Equivalence guarantee.** CLI mode and the wizard assemble
  `generate.Input` through the same shared helpers
  (`internal/tui.MountsInput` / `SmbInput` / `ParseShareFlags`), so the
  same logical inputs produce a byte-identical module tree either way
  (locked by `TestGenerate_cliTuiEquivalence_*`).

## Examples

```bash
# NFS mount with a nightly timer, written to the host overlay
dotdrift generate mounts --layer host --hostname h1 --module nas \
  --name syn01 --source synology.local:/volume1/syn01 \
  --destination /mnt/synology/syn01 --type nfs --startat "*-*-* 18:05:00"

# See which local volumes are already managed by the mounts module
dotdrift generate mounts --list-volumes

# Two shares, avahi off
dotdrift generate smb --share media=/srv/media --share data=/mnt/data --no-avahi
```


# Onboard flags (minimal)

| Flag | Default | When to use |
|------|---------|-------------|
| (none) | mode=symlink, module inferred, enabled by presence | Happy path |
| `--app ID` | inferred | Override the module directory name (default: inferred from the first path) |
| `--mode symlink\|copy\|template` | symlink | Apps that rewrite configs → copy |
| `--packages P` | none | Declare distro packages in module.toml; each entry is a bare name or `name="description"` (the description becomes a TOML comment) |
| `--tools T` | none | Declare mise tool (comma-separated or repeated for several) |
| `--host` | base module files | Host overlay only |
| `--dry-run` | false | Preview only |
| `-v`, `--verbose` | false | Stream the mise install/dotfiles output live instead of capture-and-discard, echoing each command line `set -x`-style (`+ <argv>`) to stderr before it runs (also `DD_VERBOSE=1`) |

Re-onboarding an existing module **updates** it (refresh files + merge module.toml); there is no `--force` and no conflict error.
Forbidden: `--enable`, `--resume`.

# Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime failure needing user action (e.g. mise conflict, package error) |
| 2 | Usage / invalid args |