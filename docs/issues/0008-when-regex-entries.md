---
type: Issue
title: Regex entries in when.packages and when.tools
description: Pattern matching against installed packages and mise tools, e.g. packages = ["apollo.*"].
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0008: Regex entries in when.packages and when.tools

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [profile, when-filters, packages, tools]
- **Assignee**: none
- **Related**: [issue 0006](0006-when-packages-condition.md), [issue 0007](0007-when-expression-combinators.md), [profile layout — the `[when]` filter](../product/profile-layout.md)
- **Related code**: [`internal/profile/`](../../internal/profile/), [`internal/packages/`](../../internal/packages/)
- **Closing commits**: pending

## Summary

`when.packages` / `when.tools` list entries accept regexes as well as exact names: `packages = ["apollo.*"]` matches both `apollo` and `apollo-cuda-git`.

## Details

Requested shape:

```toml
[when]
packages = ["apollo.*"]   # matches apollo AND apollo-cuda-git
```

Semantics:

- **Exact hit wins**: an installed name exactly equal to the entry satisfies it immediately — entries are found by lookup first, never re-interpreted as a pattern (an installed package literally named `apollo.*` matches the entry `apollo.*`).
- **Regex fallback**: an entry containing regex metacharacters (`. + * ? ( ) | [ ] { } ^ $ \`) that has no exact hit is matched as an **anchored full-name regex** (`^(?:entry)$`) against the installed set — `apollo.*` matches `apollo` and `apollo-cuda-git` but not `xapollo`; `apollo.+` does not match bare `apollo`. Metachar-free entries keep pure exact-match semantics and cost nothing extra.
- **Validation**: every entry must be a valid regex — a syntax error (`apollo[`) is a load-time error naming the module and carrying the entry, at any combinator depth. Literal regex-special names are written escaped: `g\+\+` matches the package `g++` (and equals it exactly when installed).
- **Probing**: a regex cannot be answered by `IsInstalled(name)`. When any regex package entry exists anywhere in the profile, the probe switches to **one installed-list query** (`Backend.Installed`: `pacman -Qq` / `dpkg-query -W -f ${Package}` / `rpm -qa --qf %{NAME}`) — a list answers plain entries too, so mixed profiles still cost one subprocess. Tool regex entries likewise trigger one `mise ls --json` call (its output is an object keyed by tool name). When the list is unavailable (unknown backend, no mise, query failure), plain entries fall back to exact per-name probes and regex entries fail open — same fail-open contract as before. Plain-only profiles keep the per-name path and probe nothing new.
- Works at any expression depth (inside `or`/`and`/`not`) — the leaf collector partitions plain vs regex entries across the whole tree.

Implementation: `packages.Backend` gains `Installed(ctx) ([]string, error)` (Paru/Apt/Dnf implement; noop errors → fail-open); `internal/profile/probes.go` gains `isRegexEntry`/`anchoredRegex`, the `probeInstalledPackageList`/`probeInstalledToolList` seams, and a `whenCollector` partitioning leaves plain/regex; `profile.installedMatch` does exact-then-anchored-regex; `validateWhen` compile-checks every entry.

## Acceptance Criteria

- [x] `packages = ["apollo.*"]` matches `apollo` and `apollo-cuda-git`; anchoring is full-name (no `xapollo`).
- [x] Same for `tools` (one `mise ls --json` probe).
- [x] Exact installed hit always wins; metachar-free entries unchanged.
- [x] Invalid regex entries are load-time errors naming module + entry, any depth; escaped forms (`g\+\+`) match literal specials.
- [x] Regex presence switches probing to one list query; list failure falls back to exact probes for plain entries; plain-only profiles unchanged.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Regex support on non-installed-state leaves (`hosts`/`users`/`os`/`gpu`/`kernel`) — exact match remains their contract.
- Version-aware tool matching (`mise current` output comparison) — presence only.
- Negated regex inside one entry (`^(?!...)` lookahead) — Go regexp does not support lookaround; use `not = { ... }`.

## Notes

- Verified live on the dev host: `pacman -Qq` (2464 names) and `mise ls --json` (57 tools) power the list probes; the `apollo.*` scenario is pinned by `TestWhenRegex_packages`.
- Tests: `internal/packages/installed_test.go` (per-backend argv + parse + error propagation), `internal/profile/regex_test.go` (anchoring, exact-wins, escaped literals, mixed/nested, validation), `TestLoad_regexEntriesUseListProbe` (probe partition + fallback), treeBackend/recording/deps/status fakes extended with `Installed` stubs.
