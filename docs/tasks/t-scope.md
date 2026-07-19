---
type: Task
title: T-scope
description: Module-level user/system dotfile scope with a sudo-applied dotfiles-system pipeline step.
tags: [task, tdd, mise, scope]
timestamp: 2026-07-19T00:00:00Z
milestone: m12
---

# Goal

Implement [scope](/milestones/m12-scope.md): a top-level `scope` key in
`module.toml` that routes the module's dotfiles through the user apply path
(default) or a root-privileged `dotfiles-system` step.

# Tests first

- `TestModuleScope_systemParses` / `_userParses` / `_omittedDefaultsToUser` —
  scope parsing and the user default (`profile.ScopeOrDefault`).
- `TestResolveScope_entriesCarryModuleScope` / `_mixedModulesPartition` —
  every `DotfileEntry` carries its module's scope; mixed profiles partition.
- `TestResolveScope_invalidScopeErrors` / `_invalidScopeErrorsWithoutDotfiles`
  — unknown scope is a resolve-time error naming module and value, even for
  modules with no dotfiles.
- `TestResolveScope_existingFixturesDefaultToUser` — scope-less profiles keep
  working unchanged.
- `TestGenerateDotfiles_systemEntriesRoundTrip` — system entries generate a
  valid standalone config (BurntSushi TOML round-trip).
- `TestDotfilesApplyArgv_nonRootUsesSudo` / `_rootSkipsSudo` / `_yesOmitted` —
  the pure argv decision: `sudo -E <mise> dotfiles apply --cd <dir> [--yes]`
  when non-root, direct when EUID 0.
- `TestExecMise_dotfilesApplySudo_invocationArgv` (table over an injectable
  euid seam) / `_trustsGeneratedConfigDir` — capturing-fake argv and trust
  env on the real exec path.
- `TestDotfilesSystemStep_appliesSystemEntries` / `_emptyEntriesNoop` /
  `_nilExecErrors` — step behavior (own config dir, sudo entry point,
  second-line empty no-op).
- `TestApply_dotfilesSystemStep` — step present iff system entries exist,
  ordered after `dotfiles`, per-scope config partitioning, D8a full config
  still complete, state records `dotfiles-system`.
- `TestApply_noSystemEntriesSkipsDotfilesSystem` — no step, no config dir, no
  state entry for user-only plans.
- `TestCLI_plan_scopeMarker` / `TestCLI_plan_jsonScope` — `[system]` text
  marker and the JSON `scope` field.
- `TestStatus_*` (updated) — progress denominator is 6.

# Implementation notes

- `profile.ModuleConfig` gains `Scope string` (`toml:"scope"`) with
  `ScopeUser`/`ScopeSystem` constants and `ScopeOrDefault()` (empty → user).
  Validation lives in `resolve.Resolve` per selected module (base layer only,
  like `id`/`app`), erroring `module %s: unknown scope %q (valid: user,
  system)` — the same fail-loud pattern as dotfile mode validation.
- `resolve.DotfileEntry` gains `Scope`; `mergeDotfiles` takes the validated
  module scope and stamps every entry.
- `ExecMise.DotfilesApplySudo(ctx, configPath, yes)` is additive: the
  `Runner` interface, `FakeRunner`, and `internal/onboard` are untouched. The
  argv comes from the pure `dotfilesApplyArgv(euid, ...)` decision function;
  `geteuid` is a package-level test seam. Non-root prepends `sudo -E` so the
  `MISE_TRUSTED_CONFIG_PATHS` entry (set on the child env by the existing
  trust plumbing) survives the elevation.
- `mise.DotfilesSystemStep{Exec, Entries, ConfigPath, Yes}` implements
  `apply.Step` as `dotfiles-system`; it takes the concrete `*ExecMise`
  (like `HooksStep`) because the sudo entry point is not on `Runner`.
- `cmd/apply.go` partitions `plan.Dotfiles.Entries` by scope, applies user
  entries through the existing `DotfilesStep` against a scope-filtered plan
  copy, and appends `DotfilesSystemStep` (config at
  `<state>/mise/dotfiles-system/mise.toml`) only when system entries exist.
  The pre-pipeline full config (D8a crash snapshot) still contains
  everything.
- `pipelineStepNames` gains `"dotfiles-system"` between `dotfiles` and
  `hooks-post`; the conditional-step caveat is documented on the var.
- Plan rendering: text marks `module: <id> [system]` (user scope unmarked);
  `plan --json` gains `scope` on each dotfile entry.

# Docs

- contract invariant 13; profile-layout `scope` schema; cli-surface sudo
  note; README scope subsection.

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
