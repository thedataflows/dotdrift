---
type: Issue
title: when expression combinators and or not
description: Boolean and/or/not combinators on the [when] filter, arbitrarily nested.
tags: [issue, product]
timestamp: 2026-08-23T00:00:00Z
---

# ISSUE 0007: when expression combinators and/or/not

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [profile, when-filters]
- **Assignee**: none
- **Related**: [issue 0006](0006-when-packages-condition.md), [profile layout](../product/profile-layout.md)
- **Related code**: [`internal/profile/`](../../internal/profile/)
- **Closing commits**: pending

## Summary

`[when]` becomes a boolean expression: the leaf fields (hosts/users/os/gpu/kernel/packages/tools) still AND together by default (the historical behavior when no combinator is present), and three combinators — `or = [...]` (any-of), `and = [...]` (all-of grouping), `not = {...}` (negation) — combine and nest arbitrarily deep.

## Details

Motivating shape (from the issue request):

```toml
[when]
not = { packages = ["somepackage"] }
kernel = ">= 7"
```

reads "kernel >= 7 AND somepackage NOT installed" — the node's leaves AND with its combinators.

Grammar (each sub-expression has exactly the shape of `[when]`):

```toml
[when]
# leaves AND together (empty = ignored)
or  = [ <when>, ... ]   # at least one element must match
and = [ <when>, ... ]   # every element must match (grouping for e.g. (a or b) and (c or d))
not = { <when> }        # the sub-expression must NOT match
```

Both TOML spellings decode identically: inline tables (`not = { ... }`) and dotted tables / arrays of tables (`[when.not]`, `[[when.or]]`).

Evaluation: a node matches when its leaves match AND every `and` element matches AND at least one `or` element matches (when the list is non-empty) AND the `not` target does not match. `not` over several leaves negates their conjunction (De Morgan: `not = { a, b }` = not-a OR not-b).

Validation, recursively over the tree, naming the module (fail-loud contract, same as a malformed `when.kernel`):

- a malformed `kernel` constraint is an error at every depth;
- an empty `not = {}` (negates nothing) is an error;
- an empty `or = []` / `and = []` is an error (BurntSushi decodes explicit empty arrays as non-nil empty slices, so omitted and empty stay distinguishable);
- an empty `or`/`and` element (`or = [{}]`) is an error — it would vacuously match and silently always-select.

Probing: `when.packages`/`when.tools` leaves anywhere in the tree (inside `or`/`and`/`not`) are collected for the lazy load-time probe exactly like top-level leaves, deduplicated across modules.

Skip reason stays `when filter`; no CLI, resolve, or fingerprint surface changes.

## Acceptance Criteria

- [x] `or`/`and`/`not` decode (inline and dotted spellings) and evaluate recursively; top-level plain-AND behavior unchanged (all pre-existing when tests pass untouched).
- [x] The motivating example evaluates as kernel AND NOT-package.
- [x] Nested leaves inside combinators participate in the lazy package/tool probe.
- [x] Malformed kernel constraints and empty combinator tables/elements are load-time errors naming the module.
- [x] Unknown keys inside nested when tables are strict-schema errors.
- [x] `go test ./...`, `go vet`, `golangci-lint` green.

## Out of Scope

- Leaf-level comparison operators beyond `kernel` (e.g. version-constrained packages/tools — still lists meaning "installed").
- XNOR/exotic operators — compose from and/or/not.
- Rendering the expression tree in `dotdrift modules`/`plan` output.

## Notes

- Implementation: `When.And`/`When.Or`/`When.Not` fields, recursive `eval` in `internal/profile/profile.go`, `validateWhen`/`validateWhenChild` (replacing `validateWhenKernel`), recursive `collectWhenLeaves` in `internal/profile/probes.go`.
- Tests: `internal/profile/expr_test.go` (combinator semantics, dotted spellings, validation errors, tree round-trip), `TestStrictModuleTOML_unknownKeyInNestedWhen`, `TestLoad_probesNestedWhenLeaves`, `TestStrictModuleTOML_fullSchemaDecodes` extended.
