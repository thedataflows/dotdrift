---
type: Issue
title: Wizard absorption &amp; contract #15 amendment
description: Decide what happens to generate --tui once the TUI covers mounts/smb, and draft the contract/ADR edits the absorption requires.
tags: [wayfinder, task, tui, contract]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0066: Wizard absorption &amp; contract #15 amendment

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:task]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [contract invariant 15](../product/contract.md), [M13 Generate](../milestones/m13-generate.md)
- **Related code**: [`cmd/generate.go`](../../cmd/generate.go), [`internal/generate/`](../../internal/generate/), [`internal/tui/theme.go`](../../internal/tui/theme.go)
- **Blocked by**: [0065 Full-schema editor suite design](0065-full-schema-editor-suite-design.md)
- **Closing commits**: 5e4f71f (machines to internal/generate), cb94c9a (CLI-only surface), 45d86dd (huh front-end deleted), 8060659 (contract 15 + ADR-0007), 15b31ea (docs + log)

## Question

The TUI absorbs the `generate --tui` wizard. What is the concrete
amendment plan?

- Contract 15 today pins CLI/wizard byte-equivalence via shared assembly
  helpers. After absorption, the invariant must say something like
  "CLI mode and the TUI editors assemble `generate.Input` through the
  same shared helpers" — draft the exact replacement wording, and check
  whether any other invariant (12, 13, 19) needs touching.
- The equivalence test story: `cmd/generate_tui_equivalence_test.go`
  asserts byte-identical trees; what is its successor once the wizard's
  huh front-end is gone and the TUI editors are the interactive path?
- CLI surface: does `generate --tui/--no-tui` disappear (so `generate`
  becomes CLI-only and interactivity lives in `tui`), and what happens
  to the no-flags-no-terminal actionable error path?
- Sequencing: can the wizard die in the same release the TUI ships, or
  is there a deprecation window? What marks it in docs (README, cli
  surface, M13's text)?
- ADR need: wizard absorption + the service-layer doorway are candidate
  ADRs (hard to reverse, surprising later, real trade-offs). Decide
  which ADR(s) the design set carries and draft their skeleton here so
  ticket 0067 assembles finished text.

## Decisions

Grilled in five rounds, accepted 2026-09-12:

- **D1 `generate` goes CLI-only — clean cut.** `--tui`/`--no-tui` die
  with the wizard; `generate mounts|smb` always assembles from flags;
  `--list-volumes` stays. The no-flags-no-terminal actionable error
  dissolves into ordinary required-flag validation (`validate()` already
  names every missing flag; only the "no terminal for the interactive
  wizard" clause disappears, with the wizard branch it guarded).
  Interactivity lives in `dotdrift tui` — the map's one-interactive-home
  stance. Auto-launch (bare `generate` opening a TUI flow) rejected: it
  keeps `generate` TUI-aware, exactly the coupling absorption ends.
- **D2 Machines move to `internal/generate` — the stated home.** The
  pure state machines and Input builders (`MountsWizard`/`SmbWizard`
  machine cores, `decisions.go`, `input.go`, `shared.go`:
  `MountsInput`/`SmbInput`, `ParseShareFlags`, `InvokingUser`,
  `kindForDefaults`, `PrintSummary`, `ExistingMountSources`) move out of
  `package tui` into `internal/generate`; only the huh front-end
  (`wizard_*.go` form functions, `chrome.go`) is deleted. Correction
  recorded: round 2 misstated the current location — the machines sit in
  `package tui` today (S4's helper unification put them there), so the
  vote's home requires a real move, which also reverses today's backwards
  `generate → tui` import: after this ticket `cmd` does not import
  `internal/tui` at all, and `tui → generate` (0065-D7's four layers)
  becomes the only arrow. Keeping them in `tui` rejected — the CLI would
  import the TUI package for its own flag assembly; a neutral package
  rejected as churn the layering already answers.
- **D3 Contract 15 stays #15, repointed at the prefill seam.** Amended
  in place (every cross-reference — 0065 cites it five times, this
  ticket's name — stays stable; the repo already tolerates numbering
  gaps, but 15 remains meaningful, so no hole). Replacement wording
  (draft, exact text below): one source of truth — the input a TUI
  editor prefills equals the input `generate` assembles, through the
  shared builders. Checked per the ticket's question: invariants 12, 13,
  and 19 never mention the wizard — **no other invariant changes**.
- **D4 Removal UX — kong's own error.** The flags leave the structs;
  `dotdrift generate mounts --tui` dies as an unknown-flag error from
  kong ("unexpected flag: --tui"). No deprecated stub: the repo's user is
  its author, README + `docs/log.md` carry the announcement, and a
  shim's only job is apologizing for a deletion. (Round 4 mis-cited
  cobra; the repo is kong — the answer is the same shape.)
- **D5 0066 ships the deletion.** Design here, review, then
  implementation inside this ticket (TDD where behavior changes, suite
  green, commit per task). No deprecation window: the wizard dies in
  this release, *before* 0065's editors land. The interim gap — no
  interactive mounts/smb path — is accepted consciously: the CLI flags
  assemble everything the wizard did, the profile's author is the only
  user, and the destination (`dotdrift tui` editors) is already designed
  and sequenced next.

Settled without a round (restated so the ticket is complete):

- **The 7 equivalence tests survive untouched.** They drive the state
  machines against the CLI path — both sides outlive the wizard. They
  retarget to "editor prefill vs flag assembly" when 0065's editors land
  (D9's enforcement moment); that successor is D3's new contract text.
- **M13 and `docs/tasks/t-generate.md` stay untouched.** Past
  milestones/task docs record what shipped; the wizard was M13's exit
  criterion and history is not retroactively edited. This ticket and the
  log entry record that it later died.
- **One ADR, not two** — `docs/adr/0007-generate-cli-only.md`. The
  contract-15 repoint is a consequence of the absorption, not a separate
  decision. (ADR-0005/0006 numbering confirmed; 0007 is next.)
- **Docs updated in this ticket** (AGENTS rule 2): README (wizard
  bullets at the generate section + command table row),
  `docs/product/cli-surface.md` (`--tui` flag row, mode-selection list,
  "The interactive wizard" section → absorption note),
  `docs/product/migrate-pimp-my-cachyos.md` (wizard-loop references →
  flag spellings), `docs/log.md` entry. `docs/issues/index.md` flips at
  closing.
- **huh leaves the tree if orphaned**: after the front-end dies, any
  `huh` import left in `internal/tui` (theme comments aside) decides
  `go.mod`/vendor removal — checked at implementation.

## Contract 15 — replacement text

> 15. **Generate assembly is single-path.** The flag-driven CLI and
> every interactive producer assemble mounts/smb modules through the
> same shared builders (`ParseShareFlags`, `MountsInput`/`SmbInput`,
> the volume/share state machines in `internal/generate`); for any flag
> set `ParseShareFlags` accepts, the input an editor prefills equals the
> input `generate` assembles, and the same logical inputs produce a
> byte-identical module tree. Interactive construction is write-once:
> all mounts/shares accumulate and `WriteModule` replaces
> `[mounts]`/`[smb]` wholesale; an aborted flow writes nothing.

(The final sentence carries the old invariant's write-semantics forward
unchanged — it was always about the shared writer, not the wizard.)

## ADR-0007 skeleton

`docs/adr/0007-generate-cli-only.md` — drafted here, finished text
written at implementation (ticket 0067 assembles the doc set):

- **Title**: Generate is CLI-only; `tui` is the one interactive home.
- **Status**: Proposed (Accepted when 0066 lands).
- **Context**: contract 15 pinned wizard/CLI equivalence while both
  existed; 0057's map and 0065's editor suite make `dotdrift tui` the
  single interactive surface, and keeping the wizard means keeping a
  huh front-end, mode-selection machinery (`selectGenerateMode`,
  `isTTY`, `runWizard`), and a `generate → tui` import that points
  backwards against the four-layer architecture.
- **Decision**: delete the wizard and the `--tui`/`--no-tui` flags;
  `generate mounts|smb` is strict flag mode; the pure machines and
  builders move to `internal/generate`; interactivity is `dotdrift tui`'s
  job; contract 15 repointed at the prefill seam.
- **Consequences**: kong unknown-flag errors for muscle memory; interim
  gap with no interactive mounts/smb path until the editor suite lands
  (accepted, D5); `huh` dependency leaves the tree if orphaned; the
  equivalence harness survives and retargets at editors.
- **References**: 0057 (map), 0065 (editor suite, D6/D7/D9), 0066, M13,
  contract 15 (old and new text).

## Implementation

Ordered tasks; TDD where behavior changes; `go test ./...` green and a
commit per task:

1. **Machines home** — move the pure machine cores and builders from
   `internal/tui` to `internal/generate` (D2): `decisions.go`,
   `input.go`, `shared.go`, plus the non-huh halves of
   `wizard_mounts.go`/`wizard_smb.go` (the `MountsWizard`/`SmbWizard`
   machines, their `Write`, loop/accumulate logic). Package rename,
   import fixes; cmd/generate switches to `generate.*`. Existing
   machine/builder tests move with their subjects. Pure relocation —
   suite green proves byte-identical behavior.
2. **CLI-only surface** (TDD first) — tests: `--tui` is an unknown flag
   for both subcommands; no-flags error names the required flags for
   both subcommands (no wizard clause). Then delete `TUI *bool` from
   both structs, `selectGenerateMode`/`generateModeQuery`/`isTTY`/
   `runWizard`/`wizardInvocation`, and the mode-selection call sites;
   subcommands go straight to validate + assemble + write.
3. **Delete the huh front-end** — `wizard_*.go` form functions,
   `chrome.go`, wizard-only helpers (`renderKV`/notices if nothing else
   uses them — theme.go stays: ADR-0003's registry is the shell's too).
   Drop `huh` from `go.mod`/vendor if no import remains. Suite green.
4. **Contract + ADR** — apply the contract-15 replacement text above;
   write `docs/adr/0007-generate-cli-only.md` from the skeleton
   (Status: Accepted).
5. **Docs** — README generate section + command row;
   `cli-surface.md` (remove `--tui` row, collapse the mode-selection
   list to "flags or error", replace the wizard section with a one-line
   absorption note pointing at `dotdrift tui`); migration recipe's two
   wizard references → flag spellings; `docs/log.md` entry.
6. **Close** — full `go test ./...` + `go vet`; flip this ticket and
   `docs/issues/index.md` to done with closing commits.
