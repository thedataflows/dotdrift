---
type: Task
title: T-tui-structural-systemd
description: systemd.units rows become editable — unit container rows, directive child rows with typed TOML values, directive/unit add and remove through the draft ledger.
tags: [task, tdd, tui, editing, issue-0074]
timestamp: 2026-09-15T00:00:00Z
issue: "0074"
---

# Goal

First 0074 slice ([issue 0074](/issues/0074-structural-section-editing.md)):
the systemd.units section stops being read-only. Units render as
container rows, each unit's directives as indented child rows carrying
their typed value; editing commits through the existing draft ledger and
the `EncodeSystemdSection` splice.

# Tests first

- `TestSystemd_directiveRowsRender` (container + directive row
  metadata), `TestSystemd_containerRowRefusesFieldEdit`,
  `TestSystemd_directiveEdit` (bare-string seeding, splice into the
  draft raw, dirty marker), `TestSystemd_directiveTableEdit` (inline
  table round-trip), `TestSystemd_directiveValueTypes` (number, list),
  `TestSystemd_directiveEmptyValueRefuses`, `TestSystemd_directiveAdd`
  (`Name = value`, unquoted value lands as a string),
  `TestSystemd_unitAddAndRemove` (confirm-gated), `TestSystemd_unitNameTier1`,
  `TestSystemd_golden`.
- Message-driven over the existing wsShell/cpress harness; the strict
  decode round-trip in applyEdit is the correctness proof.

# Implementation notes

- `wsRow` gains `container`; machine row paths join with `\x1f`
  (`pathKey`/`splitPath`) so TOML keys can never collide with the
  punctuation.
- Directive values render via `profile.EncodeTomlValue`; the edit seed is
  the bare string for string values, canonical TOML otherwise.
  `parseTomlValue` parses TOML-first with a plain-string fallback (the
  passthrough stays friendly: `ExecStart = /usr/bin/demo` needs no
  quoting). An empty value refuses — the encoder drops empty values, so
  committing one would silently delete the directive.
- Unit adds take a bare name (tier-1 mirrors resolve's charset:
  letters, numbers, `.`, `_`, `-`, `@`; kind derivation stays apply's
  business); `a` on a directive row adds `Name = value` into that unit.
- Adds validate through the new `validateAdd(family, addPath, input)`
  (the `key == "new"` branches leave validateField); structural families
  route through `mutateStructural` (structural.go) beside mutateField.

# Docs

- tui.md editing bullet gains the structural sentence as families land;
  log entry; issue 0074 checkboxes.

# Acceptance

- [x] Landed — commit `feat(tui): 0074 systemd.units editing`.
- [Definition of done](/engineering/definition-of-done.md) checklist complete.
