# Status reports drift for other existing accounts; apply stays single-account

> **Superseded by [ADR-0006](0006-status-account-notice.md)** — per-account
> drift sections proved too noisy in real use (sudo-dependent, cwd-sensitive
> probes; screens of unactionable findings). Status now shows a configuration
> notice instead. This ADR's apply half (apply stays single-account) stands.

The first field report of a `users/root/` overlay ("I see nothing") surfaced
a product question: who may converge another account's configuration? Two
directions were on the table:

1. **Multi-account apply** — one `dotdrift apply` converges every
   `users/<name>/` overlay whose account exists, delegating per-account work
   via `sudo -u <name> -H …`.
2. **Single-account apply** — applying for another account means running
   dotdrift as that account (e.g. `sudo dotdrift apply` selects root's
   overlays, per the merge rules).

Direction 1 was rejected: orchestrating per-account convergence means every
account needs a working mise (and a working dotdrift, if the orchestrator
re-execs), and that per-account upfront setup defeats the tool's purpose —
the install-easing promise dies if the human hand-installs tooling for every
account first. It also multiplies the design surface (per-account plans,
cross-account package conflicts, cursor state across accounts) for a
capability with no field demand beyond reporting.

We decided: **apply stays single-account** (direction 2), and **`status`
reports drift for every existing account with a user layer**. Reporting is
read-only, so it carries none of the convergence-side cost: file probes ride
the existing per-probe sudo elevation, tools probe via
`sudo -u <name> -H mise current`, and unprobeable accounts report `unknown`
without failing the run. The invoking account's report renders first and is
the reference; per-account sections omit findings identical to ones already
shown. An account that cannot itself run dotdrift gets a note recommending a
system-wide install (e.g. via mise, system scope) — applying as that account
requires it.

The rejected direction remains rejected, not deferred: if per-account
convergence ever gets real demand, it re-enters as a fresh effort, not a
resumption of this one.
