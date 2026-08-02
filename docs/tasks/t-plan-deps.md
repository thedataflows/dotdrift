---
type: Task
title: T-plan-deps
description: Optional --deps flag on plan rendering a dependency tree for install packages, with --deps-depth N recursion (default 1).
tags: [task, tdd, cli, packages]
timestamp: 2026-08-03T00:00:00Z
---

# Goal

`dotdrift plan --deps` renders a dependency tree (depth 1, direct
dependencies) for every package in `packages.install`, in both text and
`--json` output; `--deps-depth N` recurses deeper. Read-only
package-manager queries only — plan stays side-effect-free.

Two flags (never one optional-value int flag: kong would eat the positional
`modules` arg in `--deps 2` space form):

- `Deps bool` — `help:"Show dependency tree for packages in the install list"`
- `DepsDepth int` — `default:"1"`, `help:"Dependency recursion depth (requires --deps)"`

# Tests first

`internal/packages/deps_test.go` (new, package `packages_test`):

- `TestParsePacmanSiDepends` — canned output with version specs
  (`libreadline.so=8-64`), double spaces, and a `Depends On      : None`
  case → stripped names, empty for None.
- `TestParseAptDepends` — canned output with `Depends:`, `PreDepends:`,
  `|Depends:` alternative lines, `<virtual>` target,
  `Recommends:`/`Suggests:` → only direct+pre depends, no
  alternatives/virtual/recommends.
- `TestParseDnfRepoquery` — canned name list including the queried package
  itself → self dropped.
- `TestDepsTree_depthOne` — fake backend (`map[string][]string` + per-package
  call counts): children carry no grandchildren.
- `TestDepsTree_depthTwo` — grandchildren present.
- `TestDepsTree_cycleGuard` — a→b→a terminates; no infinite recursion.
- `TestDepsTree_memoized` — two parents sharing one dep query it once.
- `TestDepsTree_queryErrorUnknown` — fake error for one package → node
  `Unknown: true`, other packages still resolved (warn-and-proceed).
- `TestParu_DirectDeps_argv` / `TestApt_DirectDeps_argv` /
  `TestDnf_DirectDeps_argv` — recording `Runner` fakes assert exact argv:
  `paru -Si jq`, `apt-cache depends jq`,
  `dnf repoquery --requires --resolve --qf %{name}\n jq`.

`cmd/plan_test.go` (extend; stub `packagesFor` with a fake
`packages.Backend` whose `DirectDeps` serves a canned map, cleanup-restore
the seam as `stubApplyDeps` in `cmd/apply_test.go` does):

- `TestCLI_plan_depsTextRendering` — `--deps`: `  - jq` line followed by
  `    deps:` and `    - glibc` lines; without `--deps` output is
  byte-identical to today (assert `deps:` absent).
- `TestCLI_plan_depsDepthTwo` — `--deps --deps-depth 2`: grandchild line at
  six-space indent.
- `TestCLI_plan_depsDepthValidation` — `--deps-depth 0` errors
  `--deps-depth must be >= 1`; `--deps-depth 2` without `--deps` errors
  `--deps-depth requires --deps`.
- `TestCLI_plan_depsUnknownMarker` — backend error → line contains
  `- nope (deps unknown)`.
- `TestCLI_plan_jsonDeps` — `--json --deps`: `packages.deps[0].name == "jq"`,
  nested `deps[0].deps` present at depth 2; `--json` without `--deps`: no
  `"deps"` key under `packages`.

# Implementation notes

- `internal/packages/packages.go`: add `DirectDeps(ctx, pkg) ([]string, error)`
  to the `Backend` interface after `IsInstalled`; `noop` returns `n.err()`.
  Interface addition breaks two fakes: `fakeBackend` in
  `internal/packages/packages_test.go` and `recordingBackend` in
  `cmd/apply_test.go` — add the trivial method to both.
- `internal/packages/deps.go` (new): `PackageDeps{Name, Deps, Unknown}`;
  `DepsTree(ctx, b, pkgs, depth) []PackageDeps` — memoized DFS with a
  package-level cache (one query per unique name) and a recursion path set
  (cycle guard: pacman cycles like harfbuzz↔freetype2 must terminate).
  Depth 1 = children carry no `Deps`. Query error → zerolog warn-and-proceed
  and the node gets `Unknown: true`; the tree build never returns an error.
- Three `DirectDeps` implementations in deps.go, queries via the backend's
  `Runner.Run` (captured probe path, matching `IsInstalled`):
  - `Paru`: `paru -Si <pkg>` (paru, not pacman — covers AUR packages);
    `parsePacmanSiDepends` takes the `Depends On` line after the first `:`,
    splits on whitespace, `None` → empty, strips version specs (cut at first
    `=`, `<`, `>`), dedupes first-occurrence. Format verified live on this
    host.
  - `Apt`: `apt-cache depends <pkg>`; `parseAptDepends` matches
    `^\s+(Pre)?Depends: (\S+)`, skips `|Depends:` alternatives and `<virtual>`
    targets, ignores Recommends/Suggests/Conflicts/Breaks/Replaces. Unknown
    package exits 100 → error propagates. Format verified in
    `debian:bookworm-slim`.
  - `Dnf`: `dnf repoquery --requires --resolve --qf '%{name}\n' <pkg>`;
    `parseDnfRepoquery` drops empty lines and the queried package itself.
    Unverified against a real dnf — if the flags prove wrong on Fedora, fix
    only `Dnf.DirectDeps` argv; tree/printer layers don't change.
- `cmd/plan.go`: `PlanCmd` gains `Deps` and `DepsDepth` after `JSON`.
  Validation at the top of `Run`: `DepsDepth < 1` → error; `!Deps &&
  DepsDepth != 1` → error (kong cannot distinguish absent from default 1;
  `--deps-depth 1` alone is a harmless no-op). Wiring only when `Deps`:
  `backend := packagesFor(f.Backend)` (existing seam in `cmd/apply.go`),
  `packages.DepsTree(context.Background(), backend, plan.Packages.Install,
  c.DepsDepth)`.
- Rendering: `printPlan` and `printPlanJSON` gain a
  `deps []packages.PackageDeps` parameter (nil = don't render; the apply.go
  caller passes nil). Text: recursive `printDepTree(out, node, indent)`
  prints `<indent>- <name>` (plus ` (deps unknown)` when Unknown), then
  `deps:` and children at `indent+"  "`. Top-level stays at two-space
  indent so `  - pkg` is byte-identical when `--deps` is off.
- JSON: `planJSONDoc.Packages` gains `Deps []planJSONDep
  \`json:"deps,omitempty"\``; recursive `planJSONDep{Name, Deps, Unknown}`
  with omitempty tags — no `deps` key when `--deps` is unset.
- Dependencies of the `remove` list are intentionally not shown.
