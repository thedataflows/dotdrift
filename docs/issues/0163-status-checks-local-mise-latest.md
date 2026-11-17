---
type: Issue
title: Status checks the local mise latest link
description: Check the mise install link for tools declared as latest.
tags: [status, mise, tools]
timestamp: 2026-11-17T23:48:47Z
---

# ISSUE 0163: Status checks the local mise latest link

- **Type**: bug
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [status, mise]
- **Assignee**: agent
- **Related**: [contract invariant 17](../product/contract.md)
- **Related code**: [tool drift](../../internal/drift/drift.go), [mise probes](../../internal/mise/mise.go)
- **Closing commits**: none

## Summary

Status reports drift for tools declared as `latest` even when their active version matches mise's local `latest` link.

## Details

Mise returns a concrete active version. For example, PDFCraft returns `0.21.1` and its install directory has `latest -> ./0.21.1`. Status must compare the active install with that link instead of comparing the version with the word `latest`.

## Acceptance Criteria

- [x] An active install that matches the local link reports OK.
- [x] An active install that differs from the local link reports drift.
- [x] A missing or broken local link reports unknown.

## Out of Scope

- Checking remote releases or changing how apply installs tools.
