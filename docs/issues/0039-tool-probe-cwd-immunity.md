# ISSUE 0039: Tool probes are cwd-sensitive — a stray project mise.toml breaks status

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [status, drift, tools, mise]
- **Assignee**: none
- **Related**: [issue 0036](0036-tool-status-vague-reason.md)
- **Related code**: [`internal/mise/mise.go`](../../internal/mise/mise.go)
- **Closing commits**: e506377

## Summary

`status` probes tools with `mise current <tool>` in the process working
directory, so mise loads every config from the cwd upward — a stray or broken
`mise.toml` anywhere above the cwd (e.g. a `mise.toml` at the profile root,
mid-edit) makes every tool probe fail with
`error parsing config file: <path>`. A global-state probe must not depend on
the caller's cwd.

## Details

Field report: tools findings rendered
`mise current failed: exit status 1: mise ERROR error parsing config file: /home/cri/dotfiles/mise.toml (?)`.
Verified locally: `mise current` from a directory containing a malformed
`mise.toml` fails; the same probe from a clean cwd succeeds. `mise --cd` is
additive, not suppressive — it cannot exclude cwd configs (verified), so the
probe must run with a neutral process working directory.

Fix: `ExecMise.Current` runs the real probe with `cmd.Dir` set to the
account's home (mise's global-config view), overridable via a new
`Mise.ProbeDir` field. The test seams (`Mise.Run`/`RunContext`) keep their
shape and argv.

## Acceptance Criteria

- [ ] A broken `mise.toml` in the process cwd does not affect tool probes
- [ ] The probe evaluates the global view (home), not any project
- [ ] Existing `Current` tests (argv, trimming, sentinel wraps) unchanged

## Out of Scope

- Per-account probes (removed with issue 0038)

## Notes

TDD: internal mise test with a fake binary reporting its working directory.
Docs: contract #17, log.md.
