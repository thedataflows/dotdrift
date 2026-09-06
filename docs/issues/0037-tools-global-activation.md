# ISSUE 0037: Installed tools are not activated globally

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [apply, tools, mise]
- **Assignee**: none
- **Related**: [issue 0036](0036-tool-status-vague-reason.md), [mise bootstrap](../product/mise-bootstrap.md)
- **Related code**: [`internal/mise/step.go`](../../internal/mise/step.go), [`cmd/apply.go`](../../cmd/apply.go)
- **Closing commits**: HEAD

## Summary

A `[tools]` entry in any `module.toml` installs the tool via mise but never
activates it: the resolved versions land in the generated bootstrap config
under the state dir, so no global config references them and the binaries do
not resolve on PATH outside a project. Installation without activation.

## Details

Field report: "using tools section in any module.toml will install the tool
with mise, but not activate it globally."

mise loads `~/.config/mise/conf.d/*.toml` as additional **global** config
(verified: `mise config ls` lists such fragments under any HOME). That is the
activation seam: the tools step writes the resolved `[tools]` table to
`~/.config/mise/conf.d/dotdrift.toml` (XDG_CONFIG_HOME respected) after a
successful install. The fragment is dotdrift-owned and regenerated wholesale
per apply — hand edits are lost, consistent with every other generated
config. `config.toml` itself is never touched, so a profile-managed global
config (dotfile) does not conflict — the rejected alternative, `mise use -g`,
would mutate exactly that file.

Reconciliation: a run whose plan has no tools **removes** a stale fragment
(removed tools must stop resolving); a run with `--no-tools` does not touch
the fragment (deselected section, like every other artifact). `onboard`
leaves the fragment to the next `apply` (its tool list is not the full plan).

## Acceptance Criteria

- [ ] After `apply` with tools, the fragment exists, carries the resolved
      `[tools]`, and is marked generated-by-dotdrift
- [ ] After `apply` with an empty tools plan, a stale fragment is removed
- [ ] `--no-tools` leaves the fragment untouched
- [ ] Status probing needs no change (mise resolves the fragment itself)
- [ ] XDG_CONFIG_HOME is honored

## Out of Scope

- Tool *removal* from mise's data dir (uninstalling unused versions)
- Precedence between the fragment and a hand-managed `config.toml` declaring
  the same tool (documented mise behavior, not dotdrift's to police)

## Notes

TDD: ToolsStep fragment tests (write after install, stale removal,
no-fragment-path skip) in internal/mise. Docs: cli-surface apply row,
profile-layout [tools], CONTEXT.md placement-vs-activation term, log.md.
