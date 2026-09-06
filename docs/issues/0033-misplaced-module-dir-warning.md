# ISSUE 0033: Warn on misplaced module dirs (module.toml outside the modules/ level)

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [profile, ux]
- **Assignee**: none
- **Related**: [issue 0029](0029-superuser-user-overlay-visibility.md), [profile layout](../product/profile-layout.md)
- **Related code**: [`internal/profile/`](../../internal/profile/), [`cmd/`](../../cmd/)
- **Closing commits**: 6c15c9a

## Summary

A module directory placed at `users/<name>/<dir>` or `hosts/<host>/<dir>` —
missing the `modules/` level — is silently ignored by every code path:
discovery, the superuser skip marker (0029), multi-account status (0030), and
the orphan scan all look under `users/<name>/modules/` only. Surface such
directories as skipped with an actionable reason.

## Details

Field report: a user created `users/root/micro` and saw nothing anywhere —
not even an error — until diagnosis revealed the missing level. The 0029
visibility principle applies: what exists but cannot participate must say so.

At load, scan each `users/<name>/` and `hosts/<host>/` for direct child
directories (other than `modules/`) containing a `module.toml`; append each
to `Profile.Skipped` with reason
`misplaced: module belongs at users/<name>/modules/<dir>` (hosts likewise).
`modules` lists them natively; `plan`/`status`/`apply` emit one stderr
warning naming the count, via the shared preamble (same placement as the
0029 warning).

## Acceptance Criteria

- [ ] `users/root/micro/module.toml` (no `modules/` level) produces a skip
      entry with the expected-path reason
- [ ] Same for `hosts/<host>/<dir>`
- [ ] The layer's `modules/` dir itself and dirs without `module.toml` are
      not flagged
- [ ] `plan`/`status`/`apply` warn once naming the count
- [ ] Selection is untouched (misplaced modules never resolve)

## Out of Scope

- Auto-relocating the directory — the user moves it; the tool points
- Any `modules/<dir>`-level validation (base layout is already correct by
  construction)

## Notes

TDD: internal/profile skip-entry tests (user layer, host layer, modules-dir
itself exempt, no-module.toml exempt) and a cmd warning test. Docs:
profile-layout, cli-surface modules row, log.md.
