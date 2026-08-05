# Delegate convergence mechanics to `mise bootstrap`

mise v2026.8.2 ([release](https://github.com/jdx/mise/releases/tag/v2026.8.2))
turned `mise bootstrap` into a full declarative host-provisioning system:
`[bootstrap.packages]`, `[bootstrap.files]`/`[bootstrap.directories]`,
`[bootstrap.secrets]`, `[bootstrap.users]`/`[bootstrap.groups]`,
`[bootstrap.services]`, `[bootstrap.compose]`, `[bootstrap.linux.firewall]`,
`[bootstrap.linux.systemd.units]`, `[bootstrap.repos]`, `[bootstrap.hooks]`,
`[bootstrap.remote]`, plus `mise bootstrap plan`/`status`/`remote` with
per-resource `apply`/`status`. This overlaps the convergence mechanics of
dotdrift's `packages`, `dotfiles-system`, `mounts`, and `smb` pipeline steps,
and adds capabilities dotdrift explicitly scoped out of v0.1 (secrets, users
and groups, compose, firewall, SSH-remote multi-host, macOS).

We decided to make dotdrift a **translator + orchestrator** over `mise
bootstrap` rather than a parallel implementation. dotdrift compiles a layered
`module.toml` profile into one `mise.toml` carrying `[tools]`, `[dotfiles]`,
and `[bootstrap.*]` sections, then drives `mise bootstrap` with resume
semantics. The layered profile model — `modules/` → `hosts/` → `users/`,
`when` filters, `disable` union, per-section merge rules, cross-module
conflict detection — stays entirely in dotdrift; mise has no equivalent.

Why: dotdrift's irreducible value is the **authoring model** (layered modules,
selection, resolve/plan, conflict detection, generate TUI, onboard, resume),
not the convergence plumbing. mise now converges privileged files, accounts,
services, packages, and compose projects more completely and more safely than
dotdrift's hand-rolled backends — `[bootstrap.files]` writes atomically
through hidden sudo helpers that never expose file content in argv or logs, a
strict improvement over dotdrift's `sudo -E mise dotfiles apply` system path.
Keeping a private `internal/packages` backend set (paru/apt/dnf),
`internal/mounts` systemctl plumbing, and `internal/smb` group/user/service
choreography is duplicated effort against a better-maintained upstream.

Consequence: roughly 1100 lines of prod code delete — the entire
`internal/packages` backend set (541), `internal/mounts` activation (174),
`internal/smb` activation (268), and the system-dotfiles sudo path in
`internal/mise` (~150) — replaced by config-translation logic that emits
`[bootstrap.*]` tables (net-new, partially offsetting the deletion). The
8-step `internal/apply` pipeline collapses toward `mise bootstrap` phase
slices driven by the resume orchestrator. About 6300 lines (75% of prod code)
is irreducible and stays: `internal/resolve`, `internal/profile`,
`internal/generate`, `internal/tui`, `internal/onboard`, `internal/state`,
`internal/detect`/`internal/facts`, and the ensure + translator half of
`internal/mise`. The `MinMiseVersion` floor rises to `2026.8.2`.

Resume is the one invariant mise does not provide. mise bootstrap is
convergent (it skips anything already in its desired state), but it has no
cross-run cursor tied to a selection fingerprint or a resolved-plan content
hash — it cannot know that a module's content was edited or that host
selection changed. dotdrift keeps its resume layer
([contract](../product/contract.md) invariants 2, 10, 11) as a thin
orchestrator that decides "what changed?" then drives `mise bootstrap
--only/--skip <phases>` slices. dotdrift stays the brain; mise is the hands.

The tensions surfaced by the delegation are resolved as follows (tracked in [issue 0002](../issues/0002-delegate-convergence-to-mise-bootstrap.md) for the delegation and [issue 0003](../issues/0003-paru-mise-package-plugin.md) for the paru provider):

1. **AUR / `paru` — dotdrift ships a mise package plugin.** mise's
   `pacman:` built-in-manager is plain pacman (no AUR). dotdrift provides a
   `paru` package-manager plugin so bare package names on Arch resolve to
   `paru:<pkg>` and install through `paru -S` (pacman + AUR), preserving AUR
   support (`ntfsprogs-plus`, etc.). mise package plugins are Lua-based vfox
   plugins (see the package-plugin-development spec) with batch
   `PackageInstalled`/`PackageInstall` hooks; dotdrift exposes a
   `dotdrift paru` subcommand the Lua shim delegates to, keeping all paru
   invocation logic in Go (reusing the existing argv builder). **Open risk:**
   the plugin hard contract says "package plugins must never invoke `sudo` in
   any hook" — but paru self-elevates (it calls `sudo pacman` internally).
   Whether mise tolerates a plugin whose wrapped command self-elevates is
   unverified against v2026.8.2; if it blocks, the fallback is a
   dotdrift-native paru path outside the plugin model for AUR packages only.
2. **Resume semantics — kept.** dotdrift's resume layer (contract invariants
   2, 10, 11) survives as a thin orchestrator over `mise bootstrap --only`/
   `--skip <phases>`. mise bootstrap is convergent but has no cross-run cursor
   tied to selection fingerprint + plan content hash; dotdrift decides "what
   changed?" then drives the right phase slices.
3. **Verbosity — kept.** dotdrift's `-v` (`+ argv` echo to stderr, live
   streaming, probes always silent) wraps the `mise bootstrap` invocation at
   the dotdrift to mise boundary. Plugin-internal output is mise's concern.
4. **Package prefixes — bare by default, explicit overrides.** Bare names in
   `module.toml` assume the detected platform manager (Arch goes to the `paru`
   plugin; Debian to `apt`; Fedora to `dnf`); the resolver adds the `manager:`
   prefix during translation. An explicit prefix (`pacman:foo`, `apt:bar`)
   passes through to the named built-in or plugin manager unchanged.
5. **Schema lag — accepted.** The new `[bootstrap.*]` sections ship in
   v2026.8.2 but lag in the published JSON schema; editor validation catches
   up. Non-blocking.
6. **Trust + state-dir — unchanged.** Emitting a full `[bootstrap.*]` config
   under `$XDG_STATE_HOME/dotdrift/...` keeps the existing
   `MISE_TRUSTED_CONFIG_PATHS` dance; no new problem.

When this changes: if mise grows a structured module-overlay / selection model
equivalent to dotdrift's layered `module.toml` (presence=managed, `when`
filters, `disable` union, per-section merge), the case for dotdrift as a
separate binary weakens — revisit whether dotdrift collapses to a mise plugin
or a thin profile compiler. If the paru plugin's self-elevation is blocked by
mise (tension 1), `internal/packages`' paru slice is retained outside the
plugin model and this ADR's deletion estimate narrows by ~100 LOC.
