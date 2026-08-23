---
type: Issue
title: when.packages / when.tools installed-state conditions
description: Gate modules on packages or mise tools already installed on the running system.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0006: when.packages / when.tools installed-state conditions

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [profile, when-filters, packages, tools]
- **Assignee**: none
- **Related**: [profile layout](../product/profile-layout.md), [merge rules](../product/merge-rules.md)
- **Related code**: [`internal/profile/`](../../internal/profile/), [`internal/facts/`](../../internal/facts/)
- **Closing commits**: pending

## Summary

`module.toml`'s `[when]` gains `packages = ["name", ...]` and `tools = ["name", ...]`: a module is selected only when every listed system package (packages) / mise-managed tool (tools) is already installed on the running system. This extends the conditional-loading feature to the "currently installed packages or tools" dimension, alongside the existing `hosts`/`users`/`os`/`gpu`/`kernel` checks.

## Details

The `[when]` table conditionally loads modules but had no check against installed software. Use cases: an overlay module that should only manage a package's config *after* the user installed that package by hand; a module that must not fight a tool another mechanism owns; switching sibling modules on installed state (mirror of the `when.kernel` package-split example).

Semantics follow the established `when` contract exactly:

- **Lists** ANDed with the other `when` fields; empty/omitted means any (same as `hosts`/`users`/`os`).
- Membership against probed installed-state facts, keyed by the name exactly as written — no normalization of `aur/` markers or `manager:` prefixes; both sides use the same spelling or the filter does not match (case-sensitive exact match, like every other `when` value).
- Probed **lazily at profile load**, one query per distinct name deduplicated across all modules: `when.packages` through the detected package backend (`facts.Backend` → `packages.For(backend).IsInstalled`), `when.tools` through `mise current <tool>` (presence only, version ignored — mirroring the drift probe's unknown rule; a missing mise binary means nothing matches). A profile declaring neither probes nothing.
- Fail-open: a name that is absent, or whose status cannot be determined (unknown backend, package-manager failure, no mise), fails the filter — the module is skipped with reason `when filter`. No load-time error, because any list of strings is well-formed (contrast `when.kernel`, whose malformed constraint is a load-time error).
- Probe results land in `facts.Facts.InstalledPackages` / `InstalledTools` (`map[string]bool`, nil = nothing probed), injected verbatim by tests/callers — explicit facts are never overwritten by a probe.
- Implementation note: the tools probe re-implements the ~10-line `LookPath + mise current` lookup because `internal/mise` imports `internal/profile` (reverse dependency would cycle); noted with a `ponytail:` ceiling comment in `internal/profile/probes.go`.

Deliberately **not** version constraints ("jq >= 1.7") or negation — lists mean "all of these installed". Follow-up dimensions live in Out of Scope.

## Acceptance Criteria

- [x] `[when] packages = [...]` and `[when] tools = [...]` decode; the strict module.toml schema accepts both keys (machine-checked in `TestStrictModuleTOML_fullSchemaDecodes`).
- [x] Module selected when every listed package/tool is installed; skipped with reason `when filter` otherwise.
- [x] Empty/absent installed facts (or no mise binary for tools) never match a non-empty constraint (same as `when.kernel`).
- [x] Probing happens only for profiles declaring the respective `when` field, exactly once per distinct name across modules.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Package/tool **version** constraints (tools would need `mise current` version comparison; no current use case).
- Negation (`not_installed`) — use a sibling module gated on the complement, like the kernel split.
- Surfacing probed install state in `dotdrift detect` output (it is selection-internal, lazily probed).

## Notes

- Implementation: `internal/profile/probes.go` (`probeInstalledPackages`/`probeInstalledTools` seams + `enrichProbes`), `When.Packages`/`When.Tools` in `internal/profile/profile.go`, `Facts.InstalledPackages`/`InstalledTools` in `internal/facts`.
- Fixtures: `testdata/profiles/whenfilter/modules/pkgonly`, `toolonly`.
