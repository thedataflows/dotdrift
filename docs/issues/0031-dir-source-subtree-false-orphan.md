# ISSUE 0031: Base dir-source subtree falsely orphaned when an overlay redeclares the target

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [status, orphans, layers]
- **Assignee**: none
- **Related**: [issue 0030](0030-status-multi-account-drift.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`internal/drift/orphans.go`](../../internal/drift/orphans.go)
- **Closing commits**: 2029b3d

## Summary

A base module declaring a directory source (e.g. `symlink-each`) whose target
is **redeclared** by a user overlay (e.g. `mode = "copy"` for root) gets its
entire subtree falsely flagged as orphans — even while the invoking account's
live symlinks point into that very subtree.

## Details

Field report: with `modules/mise` declaring
`"~/.config/mise" = { source = "home/.config/mise", mode = "symlink-each" }`
and `users/root/modules/mise` redeclaring the same target with
`mode = "copy"`, `dotdrift status` flags every base file
(`home/.config/mise/config.toml`, `tasks/ollama.sh`, …) as
`not referenced by [dotfiles]` — false; the current account deploys them.

Root cause: `ReferencedPaths` folds declarations per enumerated view and only
marks the effective winner's tree. With a user overlay present and no host
layer, the only enumerated view is `[user, base]`; the overlay's redeclaration
wins the fold, so the base tree is never marked. The enumeration comment's
premise — "with any overlay present, some real machine always sits above
base" — is false: an account without a user layer (the common case) resolves
the bare-base view, which is never enumerated.

The fix applies the documented anchor rule at declaration time: a declaration
whose source is a directory at the declaring layer references that whole
subtree **regardless of view overrides** — dir trees deploy wholesale and
bare-base accounts are unenumerable, so a declaring layer's tree is always
live content. File sources keep the per-view resolution (the pinned
"shadowed on every host = orphan" behavior is untouched).

## Acceptance Criteria

- [ ] Base `symlink-each` subtree is not orphaned when a user overlay
      redeclares the same target with `copy`
- [ ] The overriding layer's own declared tree stays referenced
- [ ] `TestCheckOrphans_fileSourceResolvedPerView` ("shadowed everywhere" /
      "still resolves for a second host") keeps passing unchanged
- [ ] `TestCheckOrphans_dirSourceSubtreeReferenced` keeps passing unchanged

## Out of Scope

- The orphan scan flagging a symlinked `home` dir as an unreferenced *file*
  when a declaration references through it (separate cosmetic finding from
  the same field report)
- Whole-directory `copy` targets reporting `unknown` in the live drift
  probe (`is a directory`) — separate finding, separate issue

## Notes

Regression test: `TestCheckOrphans_dirSourceDeclaringLayerSurvivesOverride`
(replicates the field-report shape: base `symlink-each` + user overlay
`copy`). Docs: contract #17 anchor-rule wording, log.md.
