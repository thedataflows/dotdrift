# ISSUE 0009: bootstrap.services emits enabled as a string

- **Type**: bug
- **Status**: done
- **Priority**: high
- **Labels**: [mise, bootstrap, toml]
- **Assignee**: none
- **Related**: [ADR-0004](../adr/0004-delegate-convergence-to-mise-bootstrap.md), [issue 0002](0002-delegate-convergence-to-mise-bootstrap.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go)
- **Closing commits**: pending

## Summary

`GenerateBootstrapServices` emitted `[bootstrap.services]` entries as
`enabled = "enabled"` / `enabled = "disabled"` — TOML strings. mise's
bootstrap schema types `enabled` as a **boolean**, so the generated config
was rejected (or misparsed) by mise whenever the mounts or smb steps
converged services.

## Details

The translator mapped the `BootstrapService.Enabled bool` field onto the
strings `"enabled"`/`"disabled"` (mirroring dotdrift's own
`[mounts] state = "enabled"` vocabulary) instead of the booleans mise
expects. The bug shipped silently because the unit tests asserted the
string form, and no test parsed the generated TOML against mise's schema.

`state` is genuinely a `"running"`/`"stopped"` string in mise's schema and
was already correct; only `enabled` was wrong.

Fix: emit `enabled = true` / `enabled = false` (`%t` verb on the existing
bool field — no intermediate string mapping). The regression test now
asserts the exact emitted section, covering both boolean branches
(previously `Enabled: false` was never exercised).

## Acceptance Criteria

- [x] `GenerateBootstrapServices` emits `enabled = true` / `enabled = false` for every unit.
- [x] Tests assert the exact emitted `[bootstrap.services]` section, including a disabled unit (`enabled = false`).
- [x] No other emitter, test, fixture, or doc carries the string form.
- [x] `go test ./...` green; `go vet` clean.

## Out of Scope

- Parsing generated configs against mise's published JSON schema in CI
  (the vendored schema dump predates `[bootstrap.services]`; revisit when
  mise publishes an up-to-date schema).
- The `[mounts] state = "enabled"` vocabulary in module.toml — dotdrift's
  authoring format, correctly a string there.

## Notes

- Root cause: vocabulary bleed — dotdrift's module-level `state =
  "enabled"` words were reused for a mise field that wants a boolean.
- `tmp/mise.json` (a local schema dump) has no `bootstrap.services` defs,
  so schema checks against it would not have caught this either.
