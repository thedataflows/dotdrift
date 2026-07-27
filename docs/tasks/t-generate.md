---
type: Task
title: T-generate
description: dotdrift generate mounts|smb — registry, lsblk detection, escape port, renderers, idempotent writer, mounts/smb activation steps, strict CLI, and the equivalent interactive wizard.
tags: [task, tdd, generate, mounts, smb, tui]
timestamp: 2026-07-27T00:00:00Z
milestone: m13
---

# Goal

Implement [generate](/milestones/m13-generate.md): the `internal/generate`
engine (registry, volume detection, escape port, renderers, writer), the
mounts/smb activation steps, the `dotdrift generate mounts|smb` CLI with
strict mode selection, and the charmbracelet wizard that is
byte-equivalent to the CLI.

# Tests first

- Registry: `TestRegistry_embeddedLoads` (embedded schema parses),
  `TestRegistry_presetOptionsMatchReferenceScript` (golden presets vs the
  pimp-my-cachyos reference script), `TestRegistry_userOverrideMergesWholeEntry`
  / `_userOverrideUnknownTypeAdded` / `_malformedTOMLErrors` (user
  override from `$XDG_CONFIG_HOME/dotdrift/generate.toml` or
  `DOTDRIFT_GENERATE_REGISTRY`); `TestRecommend_kernelOps` /
  `_kernelSeamInjected` / `_unconstrainedEntryAlwaysEligible` /
  `_unknownFamily` / `_malformedExpressionErrors` (`recommended_if`
  evaluator, `KernelRelease` seam).
- Escape vectors: `TestEscape_vectors` (ground-truth outputs captured
  from the real `systemd-escape --path`, systemd 258),
  `TestEscape_matchesSystemdEscape` (live-binary cross-check, skips when
  `systemd-escape` is absent).
- Volume discovery: `TestLsblk_parsesRealFixture` /
  `_filtersNonPartitions` / `_skipsEmptyUUID` / `_skipsUnknownFstype` /
  `_skipsNetworkKind` / `_familyMatch` / `_marksAlreadyManaged` /
  `_commandFailurePropagates` / `_invalidJSONErrors` (`lsblkRun` seam,
  recorded + synthetic fixtures; null fstype never family-matches).
- Spec/merge: `TestProfile_mountsSmbParse` /
  `TestProfile_smbAvahiUnsetIsNil` /
  `TestProfile_noMountsNoSmb_emptySections`;
  `TestMergeMounts_wholeEntryByName`,
  `TestMergeSmb_sharesWholeEntryAndScalarReplace`,
  `TestMergeSmb_usersReplaceNotAppend`;
  `TestResolveMounts_missingSourceErrors` / `_missingDestinationErrors` /
  `_missingTypeErrors` / `_unknownStateErrors` / `_unknownTypePasses` /
  `_entriesCarryModuleLayerScope` / `_deterministicOrder`,
  `TestResolveSmb_shareMissingPathErrors` /
  `_moduleWithoutSmbContributesNothing`,
  `TestResolve_noMountsNoSmb_emptyAggregates` (regression guard on the
  existing fixture).
- Renderer goldens: `TestRender_nfsMountWithTimer_golden` /
  `_volumeMountNoTimer_golden` (byte-checked against envsubst'ed
  reference templates), `TestRender_optionsFromPresetWhenOmitted` /
  `_optionsOverrideWins` / `_afterForNetworkTypes` /
  `_sharesConf_golden` / `_sharesConf_groupFromSpec` / `_sharesConf_empty`
  / `_unitNameViaEscape`.
- Writer: `TestWriter_createsModuleAtBaseLayer` / `_createsModuleAtHostLayer`
  / `_createsModuleAtUserLayer`, `TestWriter_rerunByteIdentical` (SHA-256
  manifest), `TestWriter_preservesUnrelatedSections`,
  `TestWriter_unionsPackages`, `TestWriter_scopeSystemSet`,
  `TestWriter_smbConfSeededOnceNotClobbered`,
  `TestWriter_smbNilLeavesExistingAlone`,
  `TestWriter_staleArtifactsRemoved`, `TestWriter_unknownTypeErrors`
  (errors before any write).
- Steps: `TestSystemctlArgv_rootDirect` / `_nonRootSudo`,
  `TestMkdirArgv_rootDirect` / `_nonRootSudo` (pure argv + `geteuid`
  seam); `TestMountsStep_orderingMkdirReloadEnable` /
  `_stateDisabledUsesDisable` / `_disableToleratesAlreadyDisabled` /
  `_emptyPlanNoop` / `_mkdirFailureStopsStep` / `_ctxPropagated` /
  `_unitNameEscapesDestination`; `TestSmbStep_fullOrdering` /
  `_existingGroupNoGroupadd` / `_testparmBeforeRestart` /
  `_testparmFailureStopsStep` / `_noTTY_warnsAndSkipsSmbpasswd` /
  `_tty_runsSmbpasswd` / `_avahiDisabled_noAvahiCall` /
  `_avahiDefaultEnabled` / `_defaultGroupSmb` / `_emptyPlanNoop`.
- Wiring: `TestApply_mountsStepConditional` /
  `TestApply_smbStepConditional` / `TestApply_noMountsNoSmb_stepsAbsent`
  (conditional steps via `newMountsRunner`/`newSmbRunner` seams),
  `TestStatus_denominatorEight`, `TestCLI_plan_mountsSection` /
  `_smbSection` / `_noMountsSmbSectionsOmitted` / `_jsonMountsSmb`.
- CLI: `TestGenerateMountsCLI_happyPath` /
  `_missingDestinationLoudError` / `_rerunByteIdentical`,
  `TestGenerateSmbCLI_happyPath` / `_shareFlagParsing`,
  `TestGenerate_noFlagsNoTTY_helpfulError`,
  `TestGenerate_tuiFlagNoTTY_errors`,
  `TestGenerate_zeroFlagsTTY_invokesWizardSeam`,
  `TestGenerate_listVolumes_printsTable`, `TestGenerate_registered`.
- Wizard/equivalence: `TestWizard_networkSourceRegexValidation` /
  `_kernelRecommendedTypeIsDefault` / `_prefillFromFlags` /
  `_alreadyManagedVolumesPreMarked` / `_multipleMountsSingleWrite` /
  `_addMountValidation` (pure state machine, no TTY);
  `TestGenerate_cliTuiEquivalence_mounts` / `_smb` /
  `_smbDefaultAvahi` (byte-identical module trees via the shared
  `tui.MountsInput`/`SmbInput` helpers).

# Implementation notes

- `internal/generate/registry.go` + embedded `registry.toml`: preset
  options verbatim from the reference script with bare `uid`/`gid`
  substitution tokens; `family` groups alternatives (ntfs3/ntfs-3g);
  `recommended_if` supports only `kernel <op> <version>` with
  zero-padded numeric segment compare. The registry is a generate-time
  concern only — resolve never validates mount types against it.
- `internal/generate/escape.go`: `EscapePath` is a pure-Go
  `systemd-escape --path` port; documented divergence — the real binary
  hard-fails on non-normalized mid-path `..`, `EscapePath` normalizes.
- `internal/generate/lsblk.go`: `Volumes(ctx, reg, sources)` walks the
  `lsblk -J` device tree, keeps partitions with a UUID whose fstype the
  registry classifies as `volume` (direct type or family match), marks
  `Managed` for UUIDs already used as `UUID=` mount sources, sorted by
  UUID.
- `internal/generate/render.go`: pure renderers — `.mount` (`After=`
  only for network presets, uid/gid token expansion, options override vs
  preset), oneshot `.service` wrapper, `.timer` only with startat
  (Description references the real same-stem service unit), and
  `shares.conf` (sorted stanzas, comment fallback to share name,
  `valid users` fallback `@<group>`).
- `internal/generate/writer.go`: `WriteModule(root, Selection{Layer,
  Hostname, Username, ModuleID}, Input{Mounts, Smb, UID, GID})` replaces
  `[mounts]` wholesale (and `[smb]` when non-nil; nil means "not managed
  by this run"), unions packages, adds dotfile entries to
  `/usr/local/lib/systemd/system/` and `/etc/samba/`, sets
  `scope = "system"`, garbage-collects stale generated units (tracked +
  unit-named only), seeds `smb.conf` once, and validates all mount types
  before writing anything. `ModuleDir` is exported for the wizard's
  already-managed detection.
- `internal/mounts` / `internal/smb`: activation-only apply steps.
  mounts: `mkdir -p` destinations, one `daemon-reload`, per entry
  `enable --now` (plus `.timer` with startat) or tolerant
  `disable --now`. smb: group/membership, avahi when unset-or-true,
  `testparm -s` gate before any restart, service enablement, per-user
  `pdbedit -L` check → interactive `smbpasswd -a` on a TTY (stdlib
  char-device probe), warn-and-skip otherwise. Sudo argv mirrors the
  mise pattern (pure argv functions + `geteuid` seam; root direct,
  non-root `sudo -E`).
- `cmd/apply.go`: `pipelineStepNames` gains `mounts` and `smb` between
  `dotfiles-system` and `hooks-post`; steps are constructed only when the
  plan declares mounts/smb; status denominator becomes 8.
- `cmd/generate.go` + `generate_support.go`: strict mode selection
  (input flags → CLI, zero flags + TTY → wizard via the `runWizard`
  seam, zero flags + no TTY → actionable error, `--tui` forces the
  wizard); CLI mode assembles `generate.Input` through
  `tui.MountsInput`/`SmbInput`/`ParseShareFlags`/`ResolveWritable` — the
  same helpers the wizard uses — which is what makes the CLI⇔TUI
  equivalence testable.
- `internal/tui`: huh forms + lipgloss tab chrome over a pure
  spec-builder state machine (`MountsWizard`, `VolumeChoices`,
  `ValidateNetworkSource`, kernel-recommended defaults, flag pre-fill).
  The mounts loop accumulates and calls `WriteModule` once at the end —
  a mid-loop write would wipe earlier iterations, since `[mounts]` is
  replaced wholesale. Aborting before the final confirmation writes
  nothing.
- Dependencies: huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0 vendored
  (offline `GOFLAGS=-mod=vendor go test ./...` green).

# Docs

- ADR-0002; contract invariants 7, 14, 15; profile-layout `[mounts]` /
  `[smb]` schema; merge-rules whole-entry rows; cli-surface generate
  section; README command row + generate section;
  [migration recipe](/product/migrate-pimp-my-cachyos.md).

# Acceptance

- [Definition of done](/engineering/definition-of-done.md) checklist complete.
