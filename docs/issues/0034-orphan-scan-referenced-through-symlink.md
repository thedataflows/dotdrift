# ISSUE 0034: Orphan scan flags a referenced-through symlinked directory

- **Type**: bug
- **Status**: done
- **Priority**: low
- **Labels**: [status, orphans]
- **Assignee**: none
- **Related**: [issue 0031](0031-dir-source-subtree-false-orphan.md), [contract invariant 17](../product/contract.md)
- **Related code**: [`internal/drift/orphans.go`](../../internal/drift/orphans.go)
- **Closing commits**: 6c15c9a

## Summary

A symlink inside a module layer that a `[dotfiles]` declaration references
*through* (e.g. `home -> ../../../modules/micro/home` with
`source = "home/.config/micro"`) is itself flagged as an orphan:
`micro: home - not referenced by [dotfiles]`.

## Details

`filepath.WalkDir` does not descend into symlinked directories: the orphan
walk sees the link as a file, while `markTree` marks the paths beneath it
(through the link). Real directories are never flagged (the walk skips dirs),
but a symlink-to-dir fails `DirEntry.IsDir`, is not `module.toml`, and is not
itself in the referenced set — so it is reported. The link is a container for
referenced content, not content.

Fix: when the walk meets a symlink that resolves to a directory with at least
one referenced path beneath it, skip it. Dangling links and links nothing
references through stay flagged (dead cruft in a module dir is worth
knowing about).

## Acceptance Criteria

- [ ] A symlinked `home` whose subtree is referenced through it is not
      flagged
- [ ] A symlink to a directory nothing references through is still flagged
- [ ] A dangling symlink is still flagged

## Out of Scope

- Descending into symlinked directories to walk their real subtree (the
  target layer is walked on its own)

## Notes

TDD: `internal/drift` orphan test with a symlinked `home` container plus a
dangling-link control. Docs: contract #17, log.md.
