---
type: Milestone
title: M14 TUI
description: dotdrift tui — two-pane shell, full-schema editors, streamed apply over the service layer; the design set (issues 0057–0067) made into planned work.
tags: [milestone, tui]
timestamp: 2026-09-12T00:00:00Z
order: 14
---

# Goal

Ship `dotdrift tui`: the two-pane shell (approved prototype, issue 0063)
over the product service layer, with first-class structured editors for
every `module.toml` section and apply streamed inside the TUI with real
terminal handover. The design is settled — the map (issue 0057) and its
tickets 0060–0067 are the spec; this milestone only builds it. The design
docs: [tui](/product/tui.md), [service API](/product/service-api.md),
ADR-0008.

# Exit criteria

- **Reads first (0061-D7).** The reads areas (modules, plan, status,
  drift/diff, detect) live in `internal/service`; `cmd/` adapters render
  from them with **byte-identical output** per slice (kong flags
  unchanged); canonical text renderers (plan report, diff, status
  summary) exist only in the service layer and are shared with the TUI by
  construction (0061-D5).
- **The shell.** charm v2 two-pane shell per the approved prototype: tree
  fixed 30% (22–44 column clamp), border-color focus, one-line header,
  status-bar `help.Model` hints. Modules/Accounts/Profile groups; one
  node per module across layers (contract 1); overlay origins as
  expandable children; accounts as plain nodes; resolved-by-default
  selection with raw-on-demand; view stack with one active view; dirty
  editor state survives navigation; vim-ish keys, `?` help; keys +
  context menus only; every visible concept registered in ADR-0003's
  palette registry with adaptive `LightDark` colors.
- **The editor suite (0065).** `internal/tomlsplice` (multi-line-aware
  header grammar) with property tests: untouched sections byte-identical,
  determinism, spliced output strict-decodes; per-section encoders in
  `internal/profile` with goldens and the encode∘decode∘encode fixed
  point; the service config area (`ReadModuleLayer`/`WriteModuleLayer`:
  strict decode + raw text, splice + disk-hash conflict refusal + atomic
  write, broken file opens read-only); the editor frame (file-scoped
  drafts, three-tier validation, dirty-confirm) with adapters for
  form-shaped sections and custom models for dotfiles/when/hooks/
  systemd.units; module management dialogs (create scaffold,
  move-refuse, delete with orphan preview). Editors open only from the
  raw overlay-stack view; the tree position is the layer picker.
- **Contract 15 enforced at the seam.** The equivalence harness retargets
  to editor prefill vs `generate` flag assembly: for any flag set
  `ParseShareFlags` accepts, the mounts/smb editors prefill exactly the
  input `generate` assembles.
- **Apply inside the TUI.** The plan view gates on the session's
  `Preview()` ("N steps will take the terminal", with reasons); events
  stream into the progress view-model with ring+tick output coalescing;
  TTY steps hand over via `tea.ExecProcess` (sudo prompts and
  `interactive = true` hooks verified end-to-end); cancel kills the
  process group and names the interrupted step; a concurrent apply
  surfaces the typed already-running refusal (contract 11); the resume
  cursor semantics are contract 2 throughout.
- **Writes onboarded.** Onboard/restore/generate areas in the service
  layer; the Profile group's dialog actions and the module/layer generate
  context-menu actions run through them; `bootstrap.users` handled as
  emitted config (0065-D3) — Accounts manages `users/` overlays and shows
  ADR-0006's notice only.
- **Non-goals hold.** No command palette, no another-host preview, no
  undo, no when-tree builder (0065-D12's fog list stands).
- `go test ./...`, `go vet ./...`, `golangci-lint run ./...` green;
  charm v2 vendored, offline `GOFLAGS=-mod=vendor go test ./...` green;
  TUI state machines are pure with golden `View()` tests (teatest stays
  out — untagged).

# Tasks

1. [T-tui-reads](/tasks/t-tui-reads.md)
2. [T-tui-shell](/tasks/t-tui-shell.md)
3. [T-tui-editors](/tasks/t-tui-editors.md)
4. [T-tui-writes](/tasks/t-tui-writes.md)
5. [T-tui-apply](/tasks/t-tui-apply.md)

# Depends on

[M13](m13-generate.md); the apply session (issues 0069–0071) already
shipped under the design map.
