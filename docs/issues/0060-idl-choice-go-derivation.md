---
type: Issue
title: IDL choice &amp; Go derivation
description: Decide which formal IDL is the normative API contract and how hand-written Go stays conformed to it, before the service-layer architecture can be designed.
tags: [wayfinder, grilling, api, idl]
timestamp: 2026-09-11T00:00:00Z
---

# ISSUE 0060: IDL choice &amp; Go derivation

- **Type**: task
- **Status**: open
- **Priority**: medium
- **Labels**: [wayfinder:grilling]
- **Assignee**: none
- **Related**: [map 0057](0057-dotdrift-tui-design-map.md)
- **Related code**: none yet
- **Blocked by**: none (frontier)
- **Closing commits**: none

## Question

The contract is a **formal IDL, checked in, normative** even though the
transport is deferred and the only near-term consumer is Go (the TUI and
migrated CLI). Which IDL, and what does "the Go code conforms to it"
concretely mean?

- Candidate IDs: protobuf (`protoc`/`buf` schemas with no transport
  plugin), OpenAPI-without-server, JSON Schema, CUE, Smithy. Judge on:
  multi-language readiness (the stated goal), Go tooling story, diff/review
  ergonomics in this repo, dependency weight, and how naturally it
  expresses the domain's shapes (streaming apply events, layered-merge
  read models, TOML-schema-typed editors).
- Derivation direction: IDL → generated Go types (protoc-gen-go, no
  RPC), hand-written Go mirrored by a conformance test against the IDL,
  or Go → IDL export. What keeps them honest without a build-time codegen
  dependency if any?
- Where the IDL lives (`docs/api/`? `api/`?) and how it version-evolves.
- Does the IDL cover the *whole product* surface (init/detect/modules/
  plan/apply/status/onboard/restore/generate) since the service layer is
  a big-bang doorway — and if so, is full-surface coverage in one pass
  realistic, or does it version in sections?

Research carve-outs: this choice is independent of both research tickets
except on event/progress shapes — when judging how naturally each
candidate expresses streaming apply events, consult the findings of
[Bubbles &amp; layout inventory](0058-bubbles-layout-inventory.md) and
especially [Apply streaming &amp; TTY precedents](0059-apply-streaming-tty-precedents.md)
*if they have landed*; do not block on them.
