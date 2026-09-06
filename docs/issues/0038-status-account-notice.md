# ISSUE 0038: Replace per-account drift sections with a configuration notice

- **Type**: feature
- **Status**: open
- **Priority**: medium
- **Labels**: [status, multi-account, ux]
- **Assignee**: none
- **Related**: [issue 0030](0030-status-multi-account-drift.md), [ADR-0005](../adr/0005-status-reports-other-accounts.md), [ADR-0006](../adr/0006-status-account-notice.md)
- **Related code**: [`cmd/status.go`](../../cmd/status.go), [`internal/profile/accounts.go`](../../internal/profile/accounts.go)
- **Closing commits**: none

## Summary

0030's per-account drift sections are too noisy to be useful: sudo-dependent,
cwd-sensitive probes producing screens of `missing`/`(?)` lines for other
accounts. Replace them with a compact notice: which existing accounts have
configuration on this machine, and the exact apply command for each.

## Details

User decision (field report 2026-09-06): "running status command is so
confusing for other than current user. Remove all this nonsense, only let the
user know there is configuration for existing users on the current machine,
and instruct the command to run `sudo dotdrift apply` for root, and proper
sudo for other users."

The notice renders after the main report:

```
note: configuration exists for other accounts on this machine:
  users/root  — apply with: sudo dotdrift apply
  users/alice — apply with: sudo -iu alice dotdrift apply
  (each account needs dotdrift on its PATH — install system-wide, e.g. via mise, system scope)
```

- uid 0 account → `sudo dotdrift apply`; any other account →
  `sudo -iu <name> dotdrift apply` (login shell, so the account's own PATH
  applies — "proper sudo").
- `OtherAccounts` broadens: an account counts when `users/<name>/` holds at
  least one module OR a `dotdrift.toml` (a config-only overlay is still
  configuration for that account).
- All probing of other accounts goes away: `reportAccount`, `runAsAccount`,
  `outputErr`, and the finding dedup are removed; no sudo, no per-account
  mise, no drift lines. Selection, apply, plan, and modules are untouched;
  0029's skip visibility is untouched.

## Acceptance Criteria

- [ ] Status prints the notice with per-account apply commands; no
      `users/<name>:` drift sections
- [ ] uid-0 accounts get `sudo dotdrift apply`; others get `sudo -iu`
- [ ] An account with a config-only overlay (`dotdrift.toml`, no modules)
      appears in the notice
- [ ] No sudo or per-account probes run during status

## Out of Scope

- Per-account drift reporting in any form (rejected — ADR-0006)

## Notes

TDD: notice rendering (root vs other commands, config-only account, none
present) in cmd; OtherAccounts broaden + uid plumbing in internal/profile.
Docs: contract #17, cli-surface status row, log.md; ADR-0005 marked
superseded; issue 0030 notes the supersession.
