---
type: Reference
title: Bubbles & layout inventory
description: What the charmbracelet stack provides for the two-pane dotdrift tui — component inventory, the tree gap, split-pane layout, forms, testing, and version facts (researched 2026-09).
tags: [research, tui, bubbles]
timestamp: 2026-09-11T00:00:00Z
---

# Bubbles & layout inventory

Findings for [issue 0058](../issues/0058-bubbles-layout-inventory.md), verified
against upstream sources (proxy.golang.org, GitHub releases/source, pkg.go.dev)
in September 2026. The headline: **the charmbracelet v2 generation went stable
in early 2026**, which reshapes two answers the ticket assumed (tree gap, API
stability) and adds one decision the design must make first (v1 vs v2).

## Version landscape (the premise changed)

| module   | repo pin (go.mod)                         | latest v1 line                        | latest v2 line                   |
|----------|-------------------------------------------|---------------------------------------|----------------------------------|
| bubbletea| v1.3.6 (indirect)                         | v1.3.10 (2025-09-17; last v1)         | v2.0.9 (GA v2.0.0 on 2026-02-24) |
| bubbles  | main@2025-06-23 pseudo-version (indirect) | v1.0.0 (2026-02-10; honorary/frozen)  | v2.2.1 (2026-08-24)              |
| lipgloss | v1.1.0 (direct)                           | v1.1.0 (still current)                | v2.0.6                           |
| huh      | v1.0.0 (direct)                           | v1.0.0 (still current)                | v2.0.3                           |

All cells verified via [proxy.golang.org](https://proxy.golang.org)
`@v/list` / `@v/*.info` endpoints (e.g.
[bubbletea/v2](https://proxy.golang.org/github.com/charmbracelet/bubbletea/v2/@v/list),
[bubbles v1.0.0](https://proxy.golang.org/github.com/charmbracelet/bubbles/@v/v1.0.0.info)).

- The v2 modules declare a **new vanity module path, `charm.land/*`** —
  bubbles v2.2.1's go.mod is `module charm.land/bubbles/v2`
  ([go.mod](https://proxy.golang.org/github.com/charmbracelet/bubbles/v2/@v/v2.2.1.mod));
  v2 code must import the charm.land paths. bubbles v2 requires **Go 1.25.0**
  (repo is `go 1.26` — fine); bubbles v1.0.0 stays on the github.com path,
  needs Go 1.24.2
  ([go.mod](https://proxy.golang.org/github.com/charmbracelet/bubbles/@v/v1.0.0.mod)).
- The v1 line is maintenance-mode: bubbles v1.0.0 is an explicit "honorary
  release … stay tuned for the next major version"
  ([release](https://github.com/charmbracelet/bubbles/releases/tag/v1.0.0)),
  and bubbletea v1.3.10 predates v2 GA.
- The repo's bubbles pin (`…23b8fd6302d7`) is untagged main from 2025-06-23 —
  after [v0.21.0](https://github.com/charmbracelet/bubbles/releases/tag/v0.21.0)
  (2025-04-09), before v0.21.1 (2026-02-03, now a real tag). Its origin is
  bureaucratic: huh v1.0.0's own go.mod pins exactly that commit
  ([huh go.mod](https://raw.githubusercontent.com/charmbracelet/huh/v1.0.0/go.mod)).
  Re-pin to a tag when promoting; promotion itself is just imports +
  `go mod tidy && go mod vendor` (repo vendors; today's vendor tree carries
  only huh's subset, below).

## What compiles in this repo today

- `vendor/github.com/charmbracelet/bubbles/` has only the 9 packages huh
  v1.0.0 imports: cursor, filepicker, help, key, runeutil, spinner, textarea,
  textinput, viewport (`vendor/modules.txt`). `list`, `table`, `paginator`,
  `progress`, `timer` are **not vendored**; the first direct import adds them.
- `internal/tui` never imports bubbletea: the wizard is sequential huh forms
  (each `Run()` starts its own tea.Program), lipgloss chrome (`chrome.go`
  already composes the tab bar with `lipgloss.JoinHorizontal`), pure decision
  functions, and pure spec-builder state machines — tested as plain Go unit
  tests. No driver tests.

## Component inventory (bubbles)

Both lines have **no `tabs`** (only a rejected community
[PR #195](https://github.com/charmbracelet/bubbles/pull/195), closed unmerged
2024), **no `form`** (forms are huh), **no `layout`**. v1 has exactly 15
packages ([tree @ v1.0.0](https://github.com/charmbracelet/bubbles/tree/v1.0.0));
v2 adds `tree` and internalizes runeutil
([v2.0.0 notes](https://github.com/charmbracelet/bubbles/releases/tag/v2.0.0)).
Per-component (v1 paths; v2 renames where they matter):

- **list** — `Item` is just `FilterValue() string`; `ItemDelegate` controls
  row rendering (the main extension point); toggles for
  title/help/status-bar/filter/pagination; fuzzy filtering with swappable
  `FilterFunc` ([godoc](https://pkg.go.dev/github.com/charmbracelet/bubbles@v1.0.0/list)).
  Caveats: flat non-hierarchical filtering, no sorting, heavy but fully
  switchable chrome. Most-depended bubble (~1.5k importers).
- **table** — columns/rows + cursor, scrolling windowed by an internal
  viewport ([#428](https://github.com/charmbracelet/bubbles/issues/428) fixed
  by [#429](https://github.com/charmbracelet/bubbles/pull/429)); no sorting,
  pagination, or scrollbar; v2 rewrite still open
  ([PR #772](https://github.com/charmbracelet/bubbles/pull/772)) and drops
  the `Column`/`Row` wrappers — treat its API as unsettled.
- **viewport** — scrolling content region; v0.21.0 added horizontal scroll +
  renames (ScrollUp/Down, ScrollPercent —
  [release](https://github.com/charmbracelet/bubbles/releases/tag/v0.21.0));
  **no built-in scrollbar** (open
  [PR #536](https://github.com/charmbracelet/bubbles/pull/536) since 2024).
  The base layer for custom panes; v2 adds SoftWrap, regex highlights,
  gutters ([#823](https://github.com/charmbracelet/bubbles/pull/823)).
- **textinput / textarea** — placeholder, CharLimit, `ValidateFunc`,
  suggestions ([godoc](https://pkg.go.dev/github.com/charmbracelet/bubbles@v1.0.0/textinput));
  textarea: multiline, v2.1.0 dynamic height, v2.2.0 selection
  ([v2.2.0](https://github.com/charmbracelet/bubbles/releases/tag/v2.2.0)).
- **spinner / progress** — the apply-view pair (indeterminate vs animated
  determinate, FrameMsg-driven); v2 replaced the gradient API with
  `WithColors`/`WithColorFunc`
  ([UPGRADE_GUIDE_V2](https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md)).
  Streaming itself is tea.Cmd/Msg plumbing — no bubble does it for you.
- **help / key** — `help.Model` renders ShortHelp/FullHelp from any
  `help.KeyMap`; `key.Binding` + `key.Matches` is the keybinding vocabulary
  feeding it — the standard way to render ticket 0062's `?` help baseline.
- **paginator / timer / stopwatch / filepicker / cursor** — as documented;
  filepicker is the candidate for future pick-a-file flows.

## The tree gap — closed on v2, still open on v1

- **bubbles v2.2.0 (2026-08-21) shipped an official tree**
  ([PR #893](https://github.com/charmbracelet/bubbles/pull/893) by dlvhdr,
  closing [issue #233](https://github.com/charmbracelet/bubbles/issues/233)
  open since 2022). **Not backported to v1.**
- API (verified in [source at v2.2.1](https://github.com/charmbracelet/bubbles/blob/v2.2.1/tree/tree.go)):
  `tree.New(root *Node, w, h)`; fluent nodes `tree.Root(v).Child(...)`;
  viewport-backed scrolling with `SetScrollOff`; page/half-page/top/bottom
  navigation; Toggle/Open/Close current node; per-node styles
  (`NodeStyleFunc`/`SelectedNodeStyleFunc`/parent/root) and
  `Enumerator`/`Indenter` (classic/rounded, delegated to lipgloss's tree);
  embedded `help.Model` (implements `help.KeyMap`); `NodeAtCurrentOffset()`
  resolves the selection. Node values are `any` (Stringer/string). Gaps: no
  filtering, no multi-select, and it is weeks old ("imported by 0" on
  [pkg.go.dev](https://pkg.go.dev/charm.land/bubbles/v2@v2.2.1/tree)).
- **lipgloss has shipped the tree *renderer* since v1.1.0** —
  [`github.com/charmbracelet/lipgloss/tree`](https://pkg.go.dev/github.com/charmbracelet/lipgloss@v1.1.0/tree)
  (render-only: Root/Child, Enumerator/Indenter, StyleFuncs; no selection/
  keys/scrolling). The bubbles v2 tree wraps exactly this renderer
  ([node.go](https://github.com/charmbracelet/bubbles/blob/v2.2.1/tree/node.go)):
  even on the pinned v1 stack, *rendering* a tree is one import away; only
  selection/scrolling are hand-rolled (~100–150 lines, under our tests).
  Composite-view precedent: [charmbracelet/kancli](https://github.com/charmbracelet/kancli),
  cited by the [list README](https://github.com/charmbracelet/bubbles/blob/v1.0.0/list/README.md).
- The official tree deliberately did **not** build on bubbles/list (both
  [PR #639](https://github.com/charmbracelet/bubbles/pull/639) and #893 are
  viewport-over-renderer designs); sections-inside-list would also inherit
  list's flat `FilterValue` filtering.
- Community tree libs are thin: [mariusor/bubbles-tree](https://github.com/mariusor/bubbles-tree)
  (25 stars, MIT, pushed 2026-08) is the only maintained v1-compatible
  standalone option; [Evertras/bubble-data-tree](https://github.com/Evertras/bubble-data-tree)
  is dead (2022); mistakenelf/teacup's filetree is filesystem-only, dormant
  since 2024. The official catalog
  [charm-and-friends/additional-bubbles](https://github.com/charm-and-friends/additional-bubbles)
  lists no tree component.

## Layout: split panes

- **No official layout package exists**: none in bubbles, none in
  charmbracelet/x/exp ([exp tree](https://github.com/charmbracelet/x/tree/main/exp));
  the ecosystem norm is hand-rolled lipgloss composition.
- Canonical pattern: size each pane exactly (`Width(w).Height(h)` — Width
  wraps; MaxWidth/MaxHeight/Inline clip), then
  `lipgloss.JoinHorizontal(lipgloss.Top, left, right)` (auto-pads shorter
  panes to equal height); `JoinVertical` stacks header/panes/status bar;
  measure with `lipgloss.Width/Height` (never `len()`). Gotchas: Width/Height
  are *minimums*; `Height(n)` excludes borders — budget chrome via
  `GetFrameSize()` ([get.go](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/get.go));
  `Place*` no-op when content exceeds the box
  ([join.go](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/join.go),
  [README](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/README.md)).
- Resize: `tea.WindowSizeMsg` is "automatically delivered to Update when the
  Program starts and when the window dimensions change" (SIGWINCH —
  [commands.go](https://github.com/charmbracelet/bubbletea/blob/v1.3.10/commands.go));
  the root model stores WxH, recomputes pane geometry, `SetSize()`s children.
  Official examples: [window-size](https://github.com/charmbracelet/bubbletea/tree/main/examples/window-size),
  `fullscreen`, `table-resize`, `split-editors`.
- Reference implementations: **gh-dash** (sidebar+content, on the v2 stack):
  `JoinHorizontal(lipgloss.Top, section, sidebar)`, fractional pane widths,
  heights minus chrome, border width subtracted from pane content
  ([ui.go](https://github.com/dlvhdr/gh-dash/blob/main/internal/tui/ui.go));
  **superfile** renders a "terminal too small" screen below a minimum width
  ([model_render.go](https://github.com/yorukot/superfile/blob/main/src/internal/model_render.go)).
  Community helpers add little for a fixed two-pane shell:
  [76creates/stickers](https://github.com/76creates/stickers) (flex
  row/column, ~400 stars, on `charm.land/lipgloss/v2` since 2026-04) is
  alive but optional; [bubbleboxer](https://github.com/treilik/bubbleboxer)
  is stale (2023). Width math + JoinHorizontal is the whole job.
- v2-only perk for ticket 0062's focus model: bubbletea/lipgloss v2 add
  Layer/Canvas compositing with **mouse hit detection** (`LayerHitMsg`,
  `Hittable`) and a declarative `tea.View`
  ([v2 blog](https://charm.land/blog/v2/)) — clickable panes without manual
  hit-testing.

## Forms: huh inside the shell

- huh v1.0.0 is the newest v1 tag; huh v2
  ([charm.land/huh/v2](https://pkg.go.dev/charm.land/huh/v2), v2.0.3, GA
  2026-03) tracks bubbletea v2. huh's fields (Input, Text, Confirm, Select,
  MultiSelect, Note, FilePicker) cover everything the generate wizard uses
  (`huh.NewConfirm/Input/Select/MultiSelect/Note` are all `internal/tui` uses).
- **Embedding huh in a larger bubbletea app is documented and example-backed**
  (README ["What about Bubble Tea?"](https://github.com/charmbracelet/huh/blob/v1.0.0/README.md)):
  "a `huh.Form` is just a `tea.Model`" — store it, forward messages via
  `form.Update(msg)`, render `form.View()`, branch on
  `form.State == huh.StateCompleted`, read results via `GetString`/`GetInt`.
  The [official example](https://github.com/charmbracelet/huh/blob/v1.0.0/examples/bubbletea/main.go)
  renders the form beside a status pane via `lipgloss.JoinHorizontal` — split
  layout is a supported use case; an embedded form never starts its own
  tea.Program (only standalone `Run()` does).
- What embedding costs that the wizard's full-screen `Run()` hides: explicit
  sizing (`WithWidth`/`WithHeight`) and **manual key routing** — the host
  intercepts quit keys before forwarding and routes keys to the focused pane.
  Known constrained-layout issues: select width staleness after resize (#341,
  fixed in huh v2.0.3 via [PR #747](https://github.com/charmbracelet/huh/pull/747));
  long-option wrapping open ([#769](https://github.com/charmbracelet/huh/issues/769));
  FilePicker `Zoom()` takes over the form
  ([#470](https://github.com/charmbracelet/huh/issues/470)).
- Mapping the wizard: the pure decision functions and spec-builder state
  machines (`decisions.go`, `wizard_*.go`) carry over unchanged. Full-screen
  forms become embedded huh forms or modal overlays for dialog-like flows
  (confirm/apply gates); pane-embedded section editors are likelier raw
  bubbles textinput/textarea with inline validation — the design can mix both.

## Testing

- **teatest is still experimental**: both
  [charmbracelet/x/exp/teatest](https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest)
  and [exp/teatest/v2](https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest/v2)
  have **never shipped a tagged release** — pseudo-versions only (latest
  commit 2026-09-06, actively maintained). Charm: x contains "experimental
  code that we're not ready to promise any compatibility guarantees on"
  ([Writing Bubble Tea Tests](https://charm.land/blog/teatest/)). Mandating
  driver tests against an untagged exp module is a stability risk.
- The API works if wanted: `NewTestModel`/`WithInitialTermSize`, `Send`,
  `Type`, `WaitFinished`, `FinalOutput`/`FinalModel`, `WaitFor`,
  `RequireEqualOutput` (golden). The old `FinalRenderer`/`EqualAnimation`
  helpers died with the retired `charmbracelet/teatest` repo; the v2 variant
  targets bubbletea v2 ([teatest.go](https://github.com/charmbracelet/x/blob/main/exp/teatest/teatest.go)).
- Golden files need no driver: call `View()` and compare via
  [charmbracelet/x/exp/golden](https://pkg.go.dev/github.com/charmbracelet/x/exp/golden)
  (`RequireEqual`, `testdata/*.golden`, `-update`) — the helper bubbles/
  bubbletea use for their own tests.
- The wizard's current pattern — pure state machines + unit tests +
  exact-byte render asserts (`shared_test.go`) — needs no new tooling and
  stays the test seam if the shell keeps decisions pure and renders
  deterministically.

## Implications for dotdrift

1. **v1-vs-v2 is the first design decision, and it is now real.** v2 is GA
   (2026-02-24), actively developed, and has the tree; v1 is frozen
   ("honorary release") and has none. Building on v1 means a maintenance-mode
   line plus a hand-rolled tree; on v2 it means `charm.land/*` import paths,
   Go 1.25+ (repo is 1.26 — fine), v2 renames, a younger ecosystem — and
   v2-only perks (tree component, Layer/Canvas mouse-hit panes). The wizard's
   huh v1 forms can stay until absorbed (ticket 0066) — module paths
   coexist — or migrate to huh v2 in the move.
2. **The tree gap the ticket assumed is closed on v2 only.** For the IA
   ticket: official `charm.land/bubbles/v2/tree` (nav keys, toggle/open/
   close, help integration, per-node style funcs, scrolling) vs. on v1:
   lipgloss/tree renderer + hand-rolled selection. Community tree libs are
   not a safe bet at any stack level.
3. **Tabs stay hand-rolled lipgloss on any stack** — no component exists
   upstream and the idea was rejected there; `chrome.go`'s tab bar is
   already the pattern.
4. **Two-pane layout needs no new dependency**: fixed-size panes +
   `JoinHorizontal`, header/status via `JoinVertical`, resize via
   `WindowSizeMsg` → recompute → `SetSize` children — copy gh-dash's
   geometry (fractional widths, `GetFrameSize` border budgeting) and mind
   the lipgloss gotchas (Width/Height are minimums; `Place*` no-op oversize).
   Table for tabular panes has caveats (no sort/pagination, v2 rewrite
   open); viewport is the reliable base for read views.
5. **Forms**: huh remains the form layer, and embedding it in the shell is
   officially example-backed (form as child model beside a pane) — viable
   for confirm/apply-style dialogs. Editors living *inside* the right pane
   are likelier assembled from bubbles textinput/textarea with inline
   validation; either way the host owns focus routing and sizing.
6. **Testing mandate should stay pure state machines + unit tests** — the
   wizard's pattern — with plain `View()` golden-file tests (x/exp/golden)
   as the deterministic rendering check. teatest driver tests are optional,
   not a mandate, while it remains untagged.
7. **Mechanics**: promoting bubbles/bubbletea to direct deps is a one-time
   `go mod tidy && go mod vendor` with a re-pin to a tagged release; the
   current pseudo-version is inherited from huh v1.0.0's go.mod and is stale
   either way.
