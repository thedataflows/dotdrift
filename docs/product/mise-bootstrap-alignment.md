---
type: Research
title: Mise bootstrap feature alignment
description: Gap analysis between the mise bootstrap spec (mise checkout docs/bootstrap/, implemented in src/cli/bootstrap.rs + src/system/) and dotdrift's current emission/convergence code — latent misalignments, deletion opportunities, and adoptable features.
tags: [product, mise, bootstrap, research]
timestamp: 2026-09-07T00:00:00Z
---

# Sources

- Spec: `/mnt/linux2/dev/mise/docs/bootstrap/` (14 pages + 10 package-manager pages)
- Implementation: `/mnt/linux2/dev/mise/src/cli/bootstrap.rs` (4807 lines; subcommands accounts, files, services, firewall, compose, secrets, dotfiles, plugins, packages, repos, remote, linux/systemd, macos, launchd, shell, user, plan, status) and `src/system/` (accounts, files, firewall, compose, login_shell, managed_files, packages/{apk,apt,aur,brew,dnf,flatpak,mas,pacman,plugin}, …). The documented surface is implemented, including the hidden `__apply-*-plan` privileged helpers.
- dotdrift side: `internal/mise/bootstrap.go` (translators), `internal/mise/mise.go` (`Bootstrap`, `DotfilesApplySudo`), `cmd/apply.go` (steps), `internal/packages/` (own backends).

The spec is normative for dotdrift: the checkout is upstream `jdx/mise` @ `main`
(clean, f507a3c92), so every `[bootstrap.*]` emission targets exactly this
schema — there is no fork drift to excuse a mismatch.

# A. Latent misalignments (fix regardless of feature adoption)

## A1. `[bootstrap.users]` emission is rejected by mise — smb users broken

`GenerateBootstrapAccounts` (`internal/mise/bootstrap.go:250`) emits users as
`{ groups = ["<g>"], state = "present" }` — supplementary groups only. The spec
(accounts.md: "Present users require an explicit primary `group`") is enforced:
`src/system/accounts.rs:333` bails `present bootstrap user '<name>' requires a
primary group`. Any profile declaring `[smb] users = [...]` fails at apply.

**Fix:** emit `group = "<g>"` (primary) alongside `groups`. Also available:
`system`, `home`, `shell`, `comment`, `create_home`, uid/gid pinning, and
explicit `state = "absent"` + `remove_home` removal.

## A2. `Bootstrap --cd` is additive, not suppressive

`ExecMise.Bootstrap` runs `mise bootstrap --cd <dir>` from the process cwd
(`internal/mise/mise.go:632`). Issue 0039 pinned *probe* cwd, but apply still
loads cwd configs additively: a broken/stray `mise.toml` in the caller's cwd
fails every bootstrap phase. Pin `cmd.Dir` for `Bootstrap`/`DotfilesApply`/
`RunTask` the same way `runProbe` does.

## A3. System files go through `[dotfiles]` + hand-rolled sudo instead of `[bootstrap.files]`

`systemFilesStep` (`cmd/apply.go:189`) translates system-scope dotfiles back
into `[dotfiles]` entries and runs `mise dotfiles apply`, retrying elevated via
`DotfilesApplySudo`. But mise's `[bootstrap.files]` (files.md) natively
provides everything this machinery re-implements:

- try-as-user, then retry the remaining ordered changes in **one privileged
  batch** — no dotdrift-side sudo argv (`dotfilesApplyArgv`, `sudo_internal_test`
  seam) needed;
- atomic temp-file + rename writes;
- `owner`/`group`/`mode` convergence (dotdrift emits none today);
- ordering after `[bootstrap.accounts]`, so files can name accounts created in
  the same run;
- `replace = true` for node-type conflicts, `state = "absent"` (+ `recursive`)
  for explicit removal, `notify = [...]` service handlers (C1).

Adopting it deletes `DotfilesApplySudo`, the sudo argv builder, and the
try/retry pattern in `systemFilesStep`. Also aligns `status`: mise
`bootstrap files status --json` already compares content/type/mode/owner.

## A4. AUR/pacman: embedded paru plugin vs built-in managers

The fork implements `aur` (yay preferred, paru fallback;
`src/system/packages/aur.rs`) and `pacman` (`state = "absent"` supported)
natively. dotdrift instead: maps `aur/<pkg>` → `paru:` plugin, prefixes bare
Arch names with `paru:` (`PrefixedPackages`), embeds a paru plugin and copies
it into mise's registry (`internal/paru`, `paru.EnsureInstalled` in
`packagesStep.Run`), and ships `dotdrift paru install/installed` commands.

Adopting built-ins (`aur:<pkg>`, bare names → `pacman:` on Arch) deletes the
plugin embedding, the registry-maintenance step, and arguably the `paru`
command group. Behavioral deltas to weigh: built-in aur prefers **yay** over
paru; aur pins are status-only (helpers build current PKGBUILD); pacman pins
are skipped with a warning (rolling release). `packages.absent` currently runs
through dotdrift's own backends as best-effort removal; pacman entries could
instead emit `{ state = "absent" }` and gain status/drift integration (apt/dnf
have no absent support in mise — keep the own-backend path for them).

## A5. Package version pins forced to `"latest"`

`GenerateBootstrapPackages` pins everything to `"latest"`. mise supports native
pins for apt (`name=version`, `name:arch` qualifiers) and dnf, with status
reporting `version mismatch`. Adopting pins is a module.toml schema addition
(`packages.present` entries carrying a version), not just translation.

# B. Adoptable features (new capability, mapped to dotdrift concepts)

| Feature (spec page) | What it adds | Fit for dotdrift |
|---|---|---|
| **Secrets** (secrets.md) | `[bootstrap.secrets]` env-provided inputs, `{{ secret(name=…) }}` in file templates, `--prompt-secrets`, redaction from all output | **High.** smb credentials, service env files. Template mode already exists; secrets close the "passwords in git" hole. Values never in plans/logs matches dotdrift's fail-loud, no-leak posture. |
| **systemd user units** (systemd.md) | `[bootstrap.linux.systemd.units]` — user services **and timers**, `~` expansion, `dev.mise.` prefix, wide directive table | **High.** Natural `[systemd]` module section; user-scope counterpart to today's system-scope mount units. |
| **Service notify/on_change** (services.md, files.md) | Files `notify = [...]`; services `on_change` = reload/restart policy; `masked` | **Medium.** Mount/smb steps currently restart unconditionally; notify makes restarts causal. `masked` is a one-field add to `BootstrapService`. |
| **Login shell** (user.md) | `[bootstrap.user].login_shell`, `/etc/shells` handling, chsh under `SUDO_USER` | **Medium.** One-key module section; pairs with a shell module that already installs zsh/fish. |
| **Shell activation** (shell.md) | `[bootstrap.mise_shell_activate]` — marker-owned rc blocks (activate/shims), bash/zsh/fish | **Medium.** Replaces hand-rolled rc edit entries. Interplay to respect: explicit `[dotfiles]` edits win over generated blocks. Distinct from the 0037 global-tools fragment (PATH activation ≠ shell hook). |
| **Repos** (repos.md) | `[bootstrap.repos]` — declarative clones, safe-update-only, URL-normalized origin check, `update`/`exec` commands | **Medium.** Modules could declare companion checkouts (themes, plugin sources). `init` stays the profile bootstrapper. |
| **brew on Linux** (packages/brew.md) | `brew:`/`brew-cask:` managers work on Linux x86_64/arm64 with **no Homebrew install** (mise pours bottles itself) | **Medium.** Cross-distro fallback backend for packages apt/dnf/pacman lack; new backend prefix, zero plugin work. |
| **Compose** (compose.md) | `[bootstrap.compose]` — projects, config-hash drift detection, lifecycle policy, podman-compatible `command` override | **Low / defer.** Fits the home-server profile niche but is a large surface; depends_on ordering already handled by mise plan. |
| **Firewall** (firewall.md) | `[bootstrap.linux.firewall]` — nftables/firewalld/UFW backends, SSH lockout protection | **Low / defer.** Large, host-critical; revisit when a profile needs it. |
| **Package plugins** (packages/plugins.md) | `[bootstrap.plugins]` declared plugin sources | **Only if A4 is rejected.** The native way to register paru instead of dotdrift's file-copy. |

# C. Strategic divergences (note, do not adopt)

- **Remote orchestration** (remote.md): `[bootstrap.remote]`, SSH staging,
  binary provisioning, GitHub relay. dotdrift's model is one profile, local
  execution, per-host overlays. Adopting remote would invert that; the
  `hosts/<h>/` layer already answers "many machines" via git distribution.
- **macOS sections** (launchd, macos-defaults, mas): dotdrift's detect/facts
  and module conditions are Linux-shaped. No demand; exclude.
- **`mise bootstrap status/plan --json` as dotdrift's status engine**: mise now
  computes per-resource drift with origin provenance. dotdrift's `status`
  re-implements probes (dotfiles, tools, orphans). Converging them is a real
  architectural option but a large rewrite; keep as a standing consideration,
  not a ticket.

# D. Candidate issues, in priority order

1. **A1** — emit primary `group` in `[bootstrap.users]` (bug: smb users fail). → **done: [0040](../issues/0040-bootstrap-users-primary-group.md)**
2. **A2** — pin cwd for all mise invocations, not just probes (extends 0039). → **done: [0041](../issues/0041-mise-invocation-cwd-pinning.md)**
3. **A3** — move system files to `[bootstrap.files]`, delete sudo machinery. → **done: [0042](../issues/0042-system-files-bootstrap-files.md)** (sudo survives for edit entries only)
4. **A4** — adopt built-in `aur`/`pacman`, delete paru plugin + commands. → **pacman done: [0043](../issues/0043-builtin-aur-pacman-managers.md); aur + deletion gated on upstream release: [0045](../issues/0045-delete-paru-plugin-after-aur-release.md)**
5. **B-secrets** — `[bootstrap.secrets]` + template `secret()` support. → **done: [0044](../issues/0044-bootstrap-secrets-templates.md)**
6. **B-systemd-units** — `[systemd]` user units/timers module section.
7. **A5 / B-pins** — package version pins for apt/dnf.
8. **B-notify** — service `notify`/`on_change`/`masked`.
9. **B-shell** — login shell + shell activation sections.

Each adoption is its own TDD issue; this document is the map, not the work.
