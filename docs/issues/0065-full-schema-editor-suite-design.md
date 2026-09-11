---
type: Issue
title: Full-schema editor suite design
description: Design the structured editors for every module.toml section, including the unsaved-changes/validation model that keeps strict-schema and layered-merge guarantees.
tags: [wayfinder, grilling, tui, editors]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0065: Full-schema editor suite design

- **Type**: task
- **Status**: done
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: cri (main session)
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md), [profile layout](../product/profile-layout.md), [contract invariants 7, 19](../product/contract.md)
- **Related code**: [`internal/profile/spec.go`](../../internal/profile/spec.go), [`internal/tui/tui.go`](../../internal/tui/tui.go)
- **Blocked by**: [0062 TUI information architecture](0062-tui-information-architecture.md)
- **Closing commits**: 9575eb9 (design rounds + spec)

## Question

The design commits to **first-class structured editors for every
`module.toml` section** — dotfiles (whole-file AND edit entries),
packages, tools, mounts, smb, systemd.units, hooks, secrets,
`bootstrap.*`, plus module-level keys (`scope`, `disable`, `when`) and
the layer/module management operations (create/move/remove modules and
`hosts/`/`users/` layers, manage `bootstrap.users` OS accounts). How is
that suite shaped so it stays one design instead of fourteen?

- The common editor frame: field-level forms over the strict schema
  (contract 19 — unknown keys are load-time errors, so the editor can
  never emit one), section-level save semantics, whole-entry-by-name
  merge made visible when an overlay shadows a base declaration
  (contract 7).
- The unsaved-changes model: where edits live before write (in-memory
  `ModuleConfig` diff? staged TOML?), validation timing (per-field,
  per-save), and what "save" means for layered files — the editor edits
  ONE layer's `module.toml`; how the UI says which.
- Reuse of the generate spec-builders (`internal/tui`'s
  MountsWizard/SmbWizard state machines and `internal/generate`
  assembly) as the write path for mounts/smb editors, extending the
  same pattern to other sections — the absorption of the wizard starts
  here conceptually.
- The hard editors: dotfiles entries (targets, sources, modes,
  edit-entry line/block/template variants), `when` expressions
  (combinators — issues 0007/0008), secrets (env indirection). For each:
  form shape, validation story, what is deliberately deferred.
- Layer/module management: creating `hosts/<h>/modules/<m>/` vs
  `users/<u>/` overlays; moving a module between layers (what that even
  means to file layout); deleting safely (what references break).
- OS accounts (`bootstrap.users`): editing users/groups declarations vs
  the fact that they only take effect at apply.

## Decisions

Round 1 — scope, accepted 2026-09-12:

- **D1 Save semantics — textual section-splice**: a save rewrites only
  the sections being edited; every other section passes through
  verbatim (bytes, comments, position). The mechanism is onboard's
  proven splicer (`mergeModuleTOML`/`splitTOMLSections`/
  `reassembleModuleTOML`) extracted and generalized to all sections —
  BurntSushi/toml does not round-trip comments, so generate's
  whole-file encoder stays what it is today: fresh-module creation.
  Edited sections re-encode canonically (their interior comments
  re-encode away — the cost onboard already has); keyed tables emit
  sorted keys, ordered arrays (packages, hooks) preserve order; output
  is deterministic. Splice granularity is the table header: entry
  tables (`[mounts.<name>]`, `[smb.shares.<name>]`,
  `[systemd.units.<name>]`) re-encode per entry; single-table sections
  (`[dotfiles]`, `[secrets]`, `[tools]`, `[when]`, `[hooks]`)
  re-encode per section. The line-based table-header detection gains
  multi-line-string state tracking in the generalization. Decision
  followed the user's verified observation that onboard preserves
  formatting today ("keep and use this") — with the correction that
  the preserver is onboard's own splicer, not the library.
- **D2 Layer targeting — the raw view is the entry**: resolved-by-default
  selection (0062-D3) stands; an editor opens from a raw overlay-stack
  node (`base/`, `hosts/<h>/`, `users/<u>/`), and that tree position IS
  the layer choice — the editor chrome names the exact file. "Open the
  winning layer" rejected as a quiet surprise; "choose layer at save"
  rejected as dangerous (saving a resolved merge into one layer
  clobbers overlays).
- **D3 OS accounts — `users/` layers only**: `bootstrap.users` is
  *emitted* mise config derived from `[smb]` (0040), not profile
  schema — the ticket's premise is corrected. The Accounts group
  manages `users/<u>/` overlay lifecycle and shows ADR-0006's notice +
  apply command; `smb.users` stays an ordinary `[smb]` form field
  annotated "becomes OS accounts at apply". No schema change (the
  map's out-of-scope rule holds).
- **D4 Command palette — stays fog**: 0062-D6 is unchanged; the tree
  remains the only navigation model until a user story asks for a
  second one. The map's fog item notes the 0065 review.
- **D5 Module/layer management — move-refuse, orphan preview,
  scaffold**: move = wholesale module-dir move (module dirs are
  self-contained per contract 8), refused when the target layer
  already has that module; delete = confirm dialog pre-computing which
  referenced sources become orphans; create = minimal scaffold
  (`id`/`app`/`scope`) with sections added through the editors
  themselves. No content merging, ever — contract 7 semantics stay
  file-literal.

Round 2 — frame & lifecycle, accepted 2026-09-12:

- **D6 Suite frame — one schema-driven frame + section adapters** (the
  ticket's "one design instead of fourteen"): the frame owns load,
  dirty tracking, field rendering, live validation, save flow, and
  dirty-confirm chrome; an adapter declares its fields, per-field
  validators (reusing profile's own checks — never a second schema),
  entry-key handling for named tables, and its encoder. Custom view
  models only where the schema is not form-shaped: dotfiles, when,
  hooks, systemd.units. The mounts/smb adapters absorb the wizards'
  state machines (`generate.Volumes`, `kindForDefaults`, registry
  recommendations) as prefill — the conceptual start of [0066](0066-wizard-absorption-contract-15.md)'s
  absorption. Per-section hand-built editors rejected as fourteen
  validate/save stories; text-first rejected — it contradicts the
  map's first-class-structured-editors preference.
- **D7 Layering — four pieces, presentation-free below the top**:
  `internal/tomlsplice` (text mechanics: split/reassemble, header
  grammar with multi-line state; onboard refactored onto it, behavior
  unchanged); section encoders in `internal/profile` (canonical TOML
  text from typed values — CLI-reachable, so 0066's equivalence story
  shares the write path); a service config area
  (`ReadModuleLayer`/`WriteModuleLayer`: strict decode + raw text;
  validate → splice → atomic write; a broken file opens read-only with
  its contract-19 error); `internal/tui/editor` (frame + adapters,
  pure state machines per the 0058 testing mandate).
- **D8 Edit lifecycle — file-scoped drafts**: the draft unit is the
  (module, layer) *file* — raw text + baseline hash + typed
  `ModuleConfig` copy; section editors bind to the shared draft, so
  multiple dirty sections of one file save in one splice and an editor
  never hash-conflicts with itself. Dirty = deep-compare against the
  baseline; no undo in M14 (esc/`q` dirty-confirm guards; undo is
  fog). Validation in three tiers: per-field live shape checks;
  debounced in-module cross-checks (scope vs sections present,
  edit-entry field exclusivity); authoritative at save — encode →
  splice → strict-decode the spliced text (the round-trip proof:
  contract 19 by construction; a failure here is an encoder bug and a
  test-mandated invariant) → the resolve-level checks the CLI already
   enforces, run with profile context (source containment per contract
   8, source existence, cross-module target/package conflicts per
   16/9, scope constraints per 14) → disk-hash check
  (an external change refuses the save and offers reload; no merge) →
  atomic tmp+rename write.

Round 3 — editors & testing, accepted 2026-09-12:

- **D9 Per-section editors**: form adapters for module keys
  (`id`/`app`/`description`/`disabled`/`scope` with live scope
  cross-checks), packages (ordered `present` + `absent`; onboard's
  `# description` comments re-encode away when the section is edited —
  D1's named cost), tools, secrets (short form when only `env` is set,
  table otherwise), mounts (wizard state machines as prefill,
  `MountChoice.validate` reused), smb (`ShareChoice` reused; `users`
  annotated per D3). Custom models: **dotfiles** (entry list with kind
  badges; whole-file vs edit variant switch; live source-existence in
  the module dir; contract-18 field exclusivity and the
  `<file-path>/<edit-id>` key split enforced in the UI); **when** = a
  text area with live grammar validation (small grammar, positioned
  errors already exist; the structured combinator-tree builder is
  fog — user-accepted); **hooks** (ordered rows, plain-string vs
  inline-table spelling per row, `optional` checkbox; one layer's
  array only — append-across-layers is resolution, never editing);
  **systemd.units** (named key-value rows, kind badge, structural
  rules live, directives free-form passthrough).
- **D10 Module-management views**: context-menu dialogs on 0062-D4's
  view stack — create/move/delete per D5; the orphan preview reuses
  `ReferencedPaths`; the ops join the service config area.
- **D11 Testing**: tomlsplice property tests (untouched sections
  byte-identical; determinism; multi-line-string immunity; spliced
  output strict-decodes); encoder goldens plus a canonical fixed point
  (encode∘decode∘encode = encode); service conflict/atomicity/
  read-only-broken-file tests; frame state-machine + golden `View()`
  tests; adapters reuse profile's own validation vectors. The
  CLI/TUI-equivalence successor (contract 15's test) falls out as
  "adapters expose assembled values" — a seam designed here, worded in
  [0066](0066-wizard-absorption-contract-15.md).
- **D12 Fog bookkeeping**: map fog — the palette item is annotated
  "reviewed in 0065, re-deferred" (D4); added: when-editor tree
  builder (D9) and editor undo (D8); removed: "per-editor UX
  specifics" (this ticket). Nothing else moves.

## Design: the editor suite

Four layers, each with one job:

1. `internal/tomlsplice` — split a `module.toml` text into sections
   (header grammar, multi-line-string aware), reassemble in original
   order, replace one section's body. No schema knowledge.
2. `internal/profile` encoders — one per section: typed values →
   canonical TOML text. Sorted keys for keyed tables, preserved order
   for arrays.
3. Service config area — `ReadModuleLayer(root, layer, module)` →
   raw text + strict-decoded `ModuleConfig`; `WriteModuleLayer` runs
   the save pipeline and owns the disk-hash conflict check.
4. `internal/tui/editor` — the frame (draft ledger, dirty tracking,
   validation tiers, save gating, dirty-confirm chrome, `help.Model`
   bindings) plus one adapter per form-shaped section and the four
   custom models. Editors are views on 0062-D4's stack; no shell
   changes.

The save pipeline, in order: field validation green → encode draft
sections → splice into the file's raw text → strict-decode the spliced
text → resolve-level validation with profile context → disk-hash check
→ atomic tmp+rename write.

## Resolution

**The editor suite is one schema-driven frame with section adapters,
saving through onboard's splice generalized to every section.** The
unit of editing is one layer's `module.toml` file (D8), reached only
through the raw overlay-stack view (D2) — the tree position is the
layer picker, the editor names the file. Untouched sections keep their
bytes, comments, and position; edited sections re-encode canonically
and deterministically (D1), and the strict decode of the spliced text
is the round-trip proof that contract 19 holds by construction. Four
custom models cover the four sections whose schema is not form-shaped
— dotfiles variants, when as validated text, hooks ordering,
systemd passthrough (D9); everything else is a thin adapter over
profile's own validators, with the wizards' state machines living on as
mounts/smb prefill (D6). OS accounts resolve to `users/` overlay
lifecycle plus the ADR-0006 notice — `bootstrap.users` is emitted
config, not schema (D3); module management is move-refuse, orphan
preview, minimal scaffold, and never merges content (D5). The command
palette stays fog (D4), as do the when-tree builder and editor undo
(D12).

Consequences carried consciously: comments inside a saved section
re-encode away (the price of the splice, already paid under onboard);
a broken `module.toml` opens read-only rather than half-editable;
M14 ships no undo and no palette. The design unblocks
[0066 wizard absorption & contract #15 amendment](0066-wizard-absorption-contract-15.md)
(D6's prefill absorption and D11's equivalence seam feed it directly);
[0067 assemble the TUI design set](0067-assemble-tui-design-set.md)
follows once 0066 closes.
