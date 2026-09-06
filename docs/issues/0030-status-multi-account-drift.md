# ISSUE 0030: Status reports drift for other existing accounts

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [status, multi-account, ux]
- **Assignee**: none
- **Related**: [ADR-0005](../adr/0005-status-reports-other-accounts.md), [issue 0029](0029-superuser-user-overlay-visibility.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`cmd/status.go`](../../cmd/status.go), [`internal/drift/`](../../internal/drift/), [`internal/profile/`](../../internal/profile/)
- **Closing commits**: 11296dd

## Summary

`dotdrift status` reports only the invoking account's view. Extend it to also
report drift for every `users/<name>/` overlay whose `<name>` resolves to an
existing OS account, and to recommend a system-wide dotdrift install when such
an account cannot run dotdrift itself. `apply` stays single-account: applying
for another account still means running dotdrift as that account
(`sudo dotdrift apply` for root).

## Details

Field-report follow-up to 0029: the multi-account **apply** direction was
considered and rejected (per-account tool installation and PATH setup upfront
defeats the tool's purpose). What survives is multi-account **status** —
read-only reporting carries none of the apply-side cost.

Decisions (wayfinder session 2026-09-06, all recommendations user-accepted):

- **Report shape**: the invoking account's report renders as today. Then one
  section per other existing account, headed `users/<name>:`, rendering that
  account's drift. Findings identical (section, item, detail, status) to ones
  already shown in the main report are not repeated. Note: the Q1 premise
  "base+host identical across accounts" holds for the *plan*, not for the
  *drift* of `~` dotfiles — those resolve into each account's own home, so a
  base dotfile drifted in another account's home DOES surface in their
  section. Dedup of identical findings (rather than dropping shared layers
  wholesale) is what makes both true.
- **Probing**: file probes ride the existing per-probe sudo elevation (root
  reads any home); `Probes.HomeDir` is set to the account's home so `~`
  expands correctly. Tools probe as `sudo -u <name> -H <mise> current <tool>`
  using the invoking user's resolved mise binary; any failure reports
  `unknown` with detail. Non-TTY without cached sudo credentials: the
  account's checks report `unknown`; status still exits 0 (read-only
  contract, invariant 17).
- **Install recommendation**: per reported account, probe
  `sudo -u <name> -H sh -lc 'command -v dotdrift'`; when it fails, print a
  trailing note recommending a system-wide dotdrift install (e.g. via mise,
  system scope) so the account can apply.
- **Selection unchanged**: `modules`/`plan`/`apply` stay single-account;
  0029's superuser skip reason and warning still apply there. A module filter
  naming an other-account-only module still errors on the invoking view.

## Acceptance Criteria

- [ ] `dotdrift status` run as cri with a `users/root` module prints a
      `users/root:` section with that account's drift findings
- [ ] Findings already shown in the main report are not repeated in
      per-account sections
- [ ] An account whose checks cannot be probed reports `unknown`, exit 0
- [ ] A `users/<name>/` dir naming a nonexistent OS account produces no
      section
- [ ] An account with no module directories under its user layer produces no
      section
- [ ] An account without dotdrift in its login-shell PATH gets the
      system-wide-install note
- [ ] `apply`, `plan`, `modules` output and semantics unchanged

## Out of Scope

- Multi-account apply (rejected direction — see ADR-0005)
- Per-account `--diff` output (diff stays the invoking view)
- `plan`/`modules` multi-account output

## Notes

**Superseded by [0038](0038-status-account-notice.md)** (ADR-0006): the per-account drift sections built here proved too noisy in real use; status now shows a configuration notice with per-account apply commands instead.

TDD: `internal/profile` account listing (existing-with-modules listed,
nonexistent account skipped, current account excluded, module-less layer
excluded, no-users-dir tolerated) and `cmd/status` (section rendered with the
other account's drift, dedup against the main report, unknown-on-probe-
failure exit 0, install note when the account lacks dotdrift). Docs:
contract #17, cli-surface status row, log.md.
