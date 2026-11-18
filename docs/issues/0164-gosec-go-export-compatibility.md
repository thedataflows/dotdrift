---
type: Issue
title: Gosec supports Go 1.27.4 export data
description: Update the CI scanner to read Go 1.27.4 package data.
tags: [ci, gosec]
timestamp: 2026-11-18T20:05:51Z
---

# ISSUE 0164: Gosec supports Go 1.27.4 export data

- **Type**: bug
- **Status**: in-progress
- **Priority**: medium
- **Labels**: [ci, gosec]
- **Assignee**: agent
- **Related**: [failing GitHub Actions job](https://github.com/thedataflows/dotdrift/actions/runs/44384564548/job/135085454167)
- **Related code**: [CI workflow](../../.github/workflows/ci.yml)
- **Closing commits**: none

## Summary

Gosec v2.28 fails to analyze packages built with Go 1.27.4. CI needs a scanner that reads the compiler's export data.

## Details

Go 1.27.4 emits export data version 5. Gosec v2.28 uses an import reader that supports at most version 4. The resulting package import errors prevent SSA analysis. Gosec v2.30.0 uses a reader that supports version 5.

## Acceptance Criteria

- [x] The lint job passes with Go 1.27.4 and gosec v2.30.0 in act.
- [x] The gosec rule exclusions stay the same.

## Out of Scope

- Changing the project's Go version or its application code.
