# Milestones

Execute in order. Each milestone links tasks. Complete TDD acceptance before moving on.

* [M0 Scaffold](m0-scaffold.md) - Repo, CLI stubs, CI
* [M1 Profile & modules](m1-profile-modules.md) - Load, select, `modules` command
* [M2 Resolve & plan](m2-resolve-plan.md) - Merge + `plan`
* [M3 State & apply](m3-state-apply.md) - Resume cursor + pipeline fakes
* [M4 Packages](m4-packages.md) - paru/pacman backend
* [M5 Mise tools](m5-mise-tools.md) - Generate tools + install
* [M6 Mise dotfiles](m6-mise-dotfiles.md) - Generate dotfiles + apply/conflict
* [M7 Detect](m7-detect.md) - Host, user, gpu, os
* [M8 Onboard](m8-onboard.md) - Module factory + immediate mise
* [M9 Polish](m9-polish.md) - init, status, docs, v0.1 ✅
* [M10 Stretch](m10-stretch.md) - Extra backends, locks ✅

# Dependency graph

```text
M0 → M1 → M2 → M3 → M4
                 ├→ M5 → M6 → M8 → M9
                 └→ M7 (parallel after M1)
Mise ensure (tasks/t-mise-ensure) → before M5/M6/M8 integration
```
