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
   invocation logic in Go (reusing the existing argv builder). The plugin
   hard contract forbids invoking `sudo` in a hook and says mise never
   elevates for plugins — but that constrains the *plugin code* and mise's
   own elevation, not the wrapped tool: paru is a user-space binary that
   self-elevates (calls `sudo pacman` internally), so the plugin's code path
   contains no `sudo` and asks mise for none. The residual concern is merely
   whether mise gives the plugin subprocess a TTY so paru's interactive
   password prompt works — verify on a real Arch host.
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
or a thin profile compiler. If mise cannot give the paru plugin subprocess a
TTY for paru's interactive prompt (tension 1), `internal/packages`' paru
slice (~100 LOC) is retained outside the plugin model and this ADR's deletion
estimate narrows accordingly.
