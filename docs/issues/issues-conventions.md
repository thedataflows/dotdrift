---
type: Reference
title: Issue Conventions
description: Numbering, header fields, body sections, and lifecycle rules for project issues.
tags: [issue, conventions, documentation]
timestamp: 2026-08-03T00:00:00Z
---

# Issue Conventions

This document defines the conventions for issues in the dotdrift project.
An issue tracks a unit of discovered work: a bug, an emergent feature, a
chore, or a task that was not part of the original milestone plan.

Issues describe *what* needs to happen; ADRs describe *why* things are
the way they are. When an issue conflicts with an ADR, the ADR wins — fix
the issue, not the ADR.

## Scope vs. milestones and tasks

- [`docs/milestones/`](../milestones/) — ordered delivery slices for the
  original v0.1 build-out (M0–M13). Planned work.
- [`docs/tasks/`](../tasks/) — TDD task specs per milestone (tests-first
  descriptions of planned work). Planned work.
- [`docs/issues/`](.) — bugs found via dogfooding, e2e, or real use;
  features and chores that surface after the milestone plan was written;
  anything emergent. **This directory.**

If a piece of work fits a milestone's task list, it goes there. Issues
are for the rest.

## Numbering and filenames

- Issues are numbered with **4-digit zero-padded** integers: `0001`,
  `0002`, and so on. Numbers are monotonic and never reused, even if an
  issue is closed as wont-do.
- Filenames follow `NNNN-kebab-case-title.md`, for example
  `0001-cross-module-dotfile-target-conflict.md`.
- The H1 heading matches the filename title:
  `# ISSUE 0001: Cross-module dotfile target conflict`.

## Header fields

Every issue begins with a markdown bullet list of header fields, each on
its own line, using the bold-label format:

```
- **Type**: bug
- **Status**: open
- **Priority**: high
- **Labels**: [resolve, dogfooded]
- **Assignee**: none
- **Related**: [contract invariant 9](../product/contract.md)
- **Related code**: [`internal/resolve/`](../../internal/resolve/)
- **Closing commits**: none
```

- **Type** is one of `feature`, `bug`, `task`, or `chore`.
  - `feature`: new capability or behavior.
  - `bug`: existing behavior is wrong.
  - `task`: planned work that is neither (refactor, migration, cleanup).
  - `chore`: maintenance (deps, docs, tooling).
- **Status** is one of `open`, `in-progress`, `blocked`, `done`, or
  `wont-do`. `blocked` should name the blocker in **Notes**.
- **Priority** is one of `low`, `medium`, `high`, or `critical`.
- **Labels** is a short list of free-form tags. Keep the vocabulary small
  and reuse existing labels.
- **Assignee** is a person, agent, or `none`.
- **Related** links to other issues, ADRs, contract invariants, or docs,
  or `none`.
- **Related code** links to the packages or files the issue touches, as
  relative markdown links from the issue's location.
- **Closing commits** lists the short SHAs of the commits that complete
  the issue, or `none` while the issue is open. Fill it in when the issue
  moves to `done` (or `wont-do`, if commits landed before rejection).

## Body sections

Every issue has these sections, in order:

1. **Summary**: one or two sentences — what and why.
2. **Details**: enough context to start work without a meeting. Link the
   ADRs, contract invariants, or prior issues that constrain the solution.
3. **Acceptance Criteria**: a checklist of observable, verifiable
   outcomes. The issue is done when every box is checked.
4. **Out of Scope**: what this issue explicitly does not cover, so scope
   creep is visible.
5. **Notes** (optional): implementation hints, edge cases, open
   questions. Remove the section when empty.

## Lifecycle

- Issues are created as `open`.
- Move to `in-progress` when work starts, and set **Assignee**.
- Move to `done` only when every acceptance criterion is met and the
  change is merged. Record the merge's short SHAs in **Closing commits**
  in the same edit that flips the status.
- Move to `wont-do` when the work is rejected; add one line in **Notes**
  explaining why.
- Done and won't-do issues stay in the repository. They are the
  project's memory — never delete them, never reuse their numbers.

## Relationship to ADRs and the contract

An issue never changes an architectural decision. If the work requires
revisiting a decision, the issue references the ADR's "When this
changes" triggers, and a new or superseding ADR lands before (or with)
the closing commit. Likewise, an issue that changes product behavior
updates the [product contract](../product/contract.md) in the same PR —
new invariants are numbered, retired ones are struck through, never
silently rewritten.
