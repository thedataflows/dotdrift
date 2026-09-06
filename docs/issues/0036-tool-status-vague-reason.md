# ISSUE 0036: Tool status reports a vague catch-all instead of the real reason

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [status, drift, tools]
- **Assignee**: none
- **Related**: [issue 0030](0030-status-multi-account-drift.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`internal/drift/drift.go`](../../internal/drift/drift.go), [`internal/mise/mise.go`](../../internal/mise/mise.go)
- **Closing commits**: bbe5d84

## Summary

A tools-section probe failure reports
`mise not available or tool not installed (?)` — a catch-all that discards
mise's actual error and conflates two different failures. Worse, "the tool is
not installed in mise" is actionable **drift** (apply installs it), not
`unknown`.

## Details

Field report: the profile declares `"github.com:thedataflows/dotdrift" =
"latest"`; `mise current` answers `Plugin github.com/thedataflows/dotdrift is
not installed` — but `checkTools` maps any error (or empty version) to the
same `unknown` line, so the user cannot tell "mise missing" from "tool never
installed" from "probe broke".

Fix: `mise.ExecMise.Current` classifies its failures with sentinels —
`ErrUnavailable` (binary lookup failed) and `ErrNotInstalled` (mise's output
says "not installed") — preserving the underlying message. `checkTools` maps
them: not-installed → **drift** `not installed`; unavailable → unknown
`mise not available`; empty version → unknown `mise returned no version`; any
other probe failure → unknown carrying the real first error line
(`mise current failed: …`). A tool name mise's registry doesn't know stays
unknown with mise's own "not found in tool registry" message — that is a
config error, not drift, since apply cannot install it either.

## Acceptance Criteria

- [ ] mise reporting "not installed" → drift `not installed` (no `(?)`)
- [ ] mise binary missing → unknown `mise not available`
- [ ] empty version with no error → unknown `mise returned no version`
- [ ] other probe failures → unknown with the real error line
- [ ] version match/prefix-match/mismatch behavior unchanged

## Out of Scope

- Per-account (0030) tool probing gains sentinel classification too only via
  the shared error text — `sudo -u` failures surface their real stderr, but
  stay `unknown` by design (best-effort probing)

## Notes

TDD: mise sentinel tests (lookpath failure, not-installed output, pass-through
version) and drift classification tests. Docs: contract #17, cli-surface
status row, log.md.
