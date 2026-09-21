---
type: Index
title: Issue Index
description: All issues for dotdrift, most recent first.
tags: [issue, index]
timestamp: 2026-08-03T00:00:00Z
---

# Issue Index

Conventions: [issues-conventions.md](issues-conventions.md) · Template: [TEMPLATE.md](TEMPLATE.md)

Issues track *discovered* work — bugs found via dogfooding, e2e, or real
use, plus emergent features and chores. Planned work lives in
[milestones](../milestones/) and [tasks](../tasks/).

| Issue | Title | Status | Priority |
|---|---|---|---|
| [0001](0001-cross-module-dotfile-target-conflict.md) | Cross-module dotfile target conflict emits unparseable mise.toml | done | high |
| [0002](0002-delegate-convergence-to-mise-bootstrap.md) | Delegate convergence mechanics to mise bootstrap | done | high |
| [0003](0003-paru-mise-package-plugin.md) | paru mise package plugin | done | high |
| [0004](0004-resume-cursor-and-status-drift.md) | Resume-cursor-only state and status drift report | done | medium |
| [0005](0005-apply-diff-flag.md) | Apply --diff flag | done | medium |
| [0006](0006-when-packages-condition.md) | when.packages / when.tools installed-state conditions | done | medium |
| [0007](0007-when-expression-combinators.md) | when expression combinators and/or/not | done | medium |
| [0008](0008-when-regex-entries.md) | Regex entries in when.packages and when.tools | done | medium |
| [0009](0009-bootstrap-services-enabled-boolean.md) | bootstrap.services emits enabled as a string | done | high |
| [0010](0010-apply-section-flags.md) | Per-section apply flags | done | medium |
| [0011](0011-status-symlink-source-validity.md) | Status misses invalid symlink sources | done | high |
| [0012](0012-status-orphans-section.md) | Status orphans section | done | medium |
| [0013](0013-configurable-color-palette.md) | Configurable status color palette | done | low |
| [0014](0014-dir-source-subtree-orphan-false-positive.md) | Directory sources flag their own subtree as orphans | done | high |
| [0015](0015-onboard-adopts-orphans.md) | Onboard adopts orphaned module files | done | medium |
| [0016](0016-status-orphans-current-host-only.md) | Status scans orphans only for the current host and user | done | high |
| [0017](0017-onboard-adoption-units-and-profile-paths.md) | Onboard adoption claims giant ancestors and mangles profile-internal paths | done | high |
| [0018](0018-adoption-notice-layer-labels.md) | Adoption notices spell layer labels like status headings | done | low |
| [0019](0019-adopt-alias-and-readme-section.md) | adopt alias for onboard; README onboarding/adopting section | done | low |
| [0020](0020-onboard-user-flag.md) | onboard --user flag; --host --user together | done | medium |
| [0021](0021-onboard-overlay-flag-values.md) | onboard --host/--user take an optional value | done | medium |
| [0022](0022-simplify-onboard-overlay-flags.md) | Simplify onboard --host/--user to plain string flags | done | medium |
| [0023](0023-overlay-flag-empty-value-means-current.md) | onboard --host=/--user= (empty value) means the current host/user | done | medium |
| [0024](0024-restore-overlay-bool-with-value.md) | Restore bool-like --host/--user (bare flags must compose) | done | medium |
| [0025](0025-apply-backup-copy-mode-destinations.md) | apply --backup: snapshot copy-mode destinations before overwrite | done | medium |
| [0026](0026-restore-command.md) | restore command: copy backed-up copy-mode targets back to their live paths | done | medium |
| [0027](0027-readme-backup-restore-examples.md) | README: backup/restore usage examples | done | low |
| [0028](0028-apply-mise-prompts-unanswerable-stdin.md) | apply: mise confirmation prompts can never be answered (stdin is the null device) | done | high |
| [0057](0057-dotdrift-tui-design-map.md) | Dotdrift TUI design map (wayfinder) | done | high |
| [0058](0058-bubbles-layout-inventory.md) | Bubbles & layout inventory | done | medium |
| [0059](0059-apply-streaming-tty-precedents.md) | Apply streaming & TTY precedents | done | medium |
| [0060](0060-idl-choice-go-derivation.md) | IDL choice & Go derivation | done | medium |
| [0061](0061-service-layer-architecture-cli-migration.md) | Service-layer architecture & CLI migration | done | medium |
| [0068](0068-classify-install-test-isolation.md) | TestClassifyInstall writes into the real user home | done | medium |
| [0062](0062-tui-information-architecture.md) | TUI information architecture | done | medium |
| [0063](0063-two-pane-shell-prototype.md) | Two-pane shell prototype | done | low |
| [0064](0064-apply-session-tty-suspend-design.md) | Apply session & TTY suspend design | done | medium |
| [0065](0065-full-schema-editor-suite-design.md) | Full-schema editor suite design | done | medium |
| [0066](0066-wizard-absorption-contract-15.md) | Wizard absorption & contract #15 amendment | done | medium |
| [0067](0067-assemble-tui-design-set.md) | Assemble the TUI design set | done | high |
| [0069](0069-implement-apply-session-service-core.md) | Implement apply-session service core | done | high |
| [0070](0070-migrate-cmd-apply-onto-apply-session.md) | Migrate cmd/apply onto the apply session | done | high |
| [0071](0071-surface-tty-handover-for-real-steps.md) | Surface TTY handover for real steps | done | high |
| [0072](0072-module-management-dialogs.md) | Module-management dialogs in the TUI | done | high |
| [0073](0073-tui-compositor-redesign.md) | TUI compositor redesign | done | high |
| [0074](0074-structural-section-editing.md) | Structural-section inline editing in the M15 workspace | done | medium |
| [0075](0075-tui-ergonomics-paging-selection-disclosure.md) | TUI ergonomics round — paging, visible selection, render-what-is disclosure | done | high |
| [0076](0076-tui-choice-editors-and-location.md) | TUI editing round — choice pickers for closed-set fields, cursor location you can see | done | medium |
| [0077](0077-tui-add-forms.md) | TUI add forms — `a` opens a labeled form, writes becomes addable | done | high |
| [0078](0078-tui-enter-fallback-and-footer-hints.md) | TUI discoverability round — enter falls back to the add form, the footer hints name the primary verbs | done | high |
| [0079](0079-tui-undo-marks-onboard-cycle.md) | TUI intuition round — undo/redo, gesture marks, prefilled onboard, in-place choice cycling | done | high |
| [0080](0080-tui-value-label-colors.md) | TUI color semantics — values read as content, fixed labels recede | done | medium |
| [0081](0081-tui-nav-inplace-filter.md) | TUI nav quick filter — / filters the module list in place, no modal | done | medium |
| [0082](0082-tui-override-module.md) | Override module from the TUI — seeded overlays, live nav, merge-rule hint | done | medium |
| [0083](0083-ponytail-audit-application.md) | Apply the ponytail over-engineering audit — delete the dead code, one lipgloss | done | medium |
| [0084](0084-restore-dialog-plan-never-lands.md) | Restore dialog's plan resolution never lands — restorePlanMsg has no production consumer | done | high |
| [0085](0085-writes-dialogs-refresh-the-nav.md) | Writes dialogs refresh the nav — the message-flow sweep | done | medium |
| [0086](0086-module-toml-save-trailing-newline.md) | Saving a module.toml does not guarantee a trailing newline | done | medium |
| [0087](0087-tui-remove-row-dead-on-structural-rows.md) | TUI `d` (remove row) is dead on smb scalars/fields, secrets/mounts fields, when leaves, and writes block rows | done | medium |
| [0088](0088-hook-tasks-always-interactive.md) | Hook tasks are always generated `interactive = true`, not gated on the writing session's TTY | done | high |
| [0089](0089-tui-dialog-confirm-gate-invisible.md) | Form dialogs arm an invisible confirm gate — enter freezes the dialog | done | high |
| [0090](0090-delete-app-field-rename-app-flag.md) | Delete the `app` module.toml field; rename `--app` to `--module` | done | medium |
| [0091](0091-tui-text-inputs-drop-paste.md) | TUI text inputs drop bracketed paste — PasteMsg has no consumer | done | high |
| [0092](0092-tui-field-caret-glyph-shifts-text.md) | Field editor caret glyph occupies a cell — letters right of the caret shift | done | medium |
| [0093](0093-tui-yank-modules-scoped-plan-apply.md) | Nav yank — space/y marks modules, p/P plan and apply the yanked set only | done | medium |
| [0094](0094-tui-typed-inputs-multiline-filepicker.md) | Typed inputs — multi-line editor (optional coloring), file picker on enter, ctrl+o browse in forms | done | medium |
| [0095](0095-tui-links-edit-opens-link-modal.md) | Links-row edit opens the link modal (target/source pair) instead of a bare text input | done | medium |
| [0096](0096-tui-picker-opens-on-current-value.md) | File picker opens on the field's current value, its own entry selected | done | medium |
| [0097](0097-tui-picker-up-dir-row.md) | File picker lists a `../` up-dir row whenever the directory has a parent | done | medium |
