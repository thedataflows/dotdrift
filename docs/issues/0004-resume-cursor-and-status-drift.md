# ISSUE 0004: Resume-cursor-only state and status drift report

- **Type**: feature
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [state, status]
- **Assignee**: none
- **Related**: [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [contract invariants 2, 4, 10, 11](../product/contract.md)
- **Related code**: [`internal/state/`](../../internal/state/), [`internal/apply/`](../../internal/apply/), [`internal/drift/`](../../internal/drift/), [`cmd/`](../../cmd/)
- **Closing commits**: none

## Summary

The apply state file is reduced to a single value — the name of the last
successfully completed pipeline step — and is deleted when the pipeline
completes, so a successful apply leaves no state file and the next run starts
from the beginning. `dotdrift status` becomes a read-only drift report that
compares the resolved profile against the live system.

## Details

The apply state file previously carried a selection fingerprint, a resolved-plan
content hash (`PlanHash`), a `completed` map, a `current` step, a `status`, and
an `error`. Resume only ever needs "where did we stop": everything else was
unused surface that existed to force-reset completed steps when module content
edded after a failed apply. The state file is transient by design (deleted on
success), and mise-side convergence is idempotent, so dropping the content-hash
reset is an accepted tradeoff: editing a module after a failed apply no longer
force-resets already-completed steps, but a completed apply is always a clean
full run.

`status` answered the wrong question. It printed the resume cursor and last
error; the actual operational question is "does the live system match the
profile?" `status` now resolves the plan and probes the live system — packages
(installed?), tools (`mise current`), dotfiles (symlink target / copy content /
template existence), mounts (systemd unit enabled+active, destination dir),
and smb (group/users/services/shares via `getent`/`id`/`systemctl`/`testparm`)
— printing an `ok`/`drift`/`unknown` report plus the resume cursor line. It
exits 0 even when drift is found (the user decides whether to apply). Tools are
probed via `mise current` only when a mise binary resolves via `LookPath`;
otherwise every tool reports `unknown`.

## Acceptance Criteria

- [x] State JSON is exactly `{"lastCompleted": ...}`; the file is absent after a
  successful apply.
- [x] Resume skips steps through the cursor; an unknown cursor runs everything.
- [x] `status` prints drift for packages/tools/dotfiles/mounts/smb and exits 0
  with drift.
- [x] Tools report `unknown` when mise is absent.
- [x] `go test ./...` green; `go vet` clean.
- [x] Contract / cli-surface / README / package-layout / definition-of-done /
  ADR-0004 / `docs/log.md` updated.

## Out of Scope

- Hook drift (no observable desired state).
- Template content comparison (existence-only; mise renders templates,
  recomputing is out of scope).
- JSON output for `status`.
- Non-zero exit on drift (explicitly rejected — report-only).
- A module filter on `status`.
- Migration of old state files (they decode to an empty cursor → one full
  apply, then the file is deleted).

## Notes

`resolve.Fingerprint` survives for `plan` output only; `resolve.PlanHash` is
deleted. Scoped-apply caveat: a filtered apply that resumes from another run's
cursor cannot detect the scope change — delete the state file (path shown by
`dotdrift status`) to force a full run. Old pre-change state files decode with
all old fields ignored → `LastCompleted == ""` → one full apply, after which the
file is deleted; no migration code.
