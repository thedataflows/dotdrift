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

Two open questions gate the migration and are tracked in
[issue 0002](../issues/0002-delegate-convergence-to-mise-bootstrap.md):

1. **AUR / `paru`.** dotdrift's Arch backend is `paru -S` (pacman + AUR);
   mise's `pacman:` built-in-manager is plain pacman with no AUR. The
   migration doc itself references the AUR package `ntfsprogs-plus`. This must
   be resolved (keep a thin paru path, require an AUR-capable mise plugin, or
   accept AUR-only modules stay dotdrift-native) before `internal/packages`
   can be deleted.
2. **Verbose UX.** dotdrift's `-v` does `set -x`-style `+ argv` echo to stderr
   with probes always silent; mise offers `MISE_LOG_LEVEL`/`MISE_DEBUG`.
   Decide whether to keep dotdrift's echo wrapper or accept mise's native
   logging.

When this changes: if mise grows a structured module-overlay / selection model
equivalent to dotdrift's layered `module.toml` (presence=managed, `when`
filters, `disable` union, per-section merge), the case for dotdrift as a
separate binary weakens — at that point revisit whether dotdrift collapses to
a mise plugin or a thin profile compiler. Conversely, if the AUR question
cannot be solved inside mise, `internal/packages` (or a paru slice of it) is
retained and this ADR's deletion estimate narrows.
