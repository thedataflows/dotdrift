---
type: Issue
title: "TICKET: service notify, on_change, and masked"
description: Decision ticket — whether mount/smb unit emissions gain notify-on-file-change causality and masked support, or unconditional restarts stay.
tags: [issue, wayfinder:grilling, mise, services]
timestamp: 2026-09-07T00:00:00Z
---

# ISSUE 0050: TICKET — service notify, on_change, and masked

- **Type**: feature
- **Status**: open
- **Priority**: low
- **Labels**: [wayfinder:grilling, mise, services]
- **Assignee**: none
- **Related**: [map 0047](0047-mise-bootstrap-adoptions-map.md), [alignment B-notify](../product/mise-bootstrap-alignment.md)
- **Related code**: [`internal/mise/bootstrap.go`](../../internal/mise/bootstrap.go), [`cmd/apply.go`](../../cmd/apply.go)
- **Closing commits**: none

## Question

Do dotdrift's `[bootstrap.services]` emissions (mount units, smb/avahi) adopt
mise's causal restart model — managed files `notify = [...]`, services
`on_change = reload|restart|reload_or_restart|none` — and/or `masked`?

Open decisions:

1. **Need evidence** — today the services phase converges state/enabled only;
   config file changes (smb.conf, unit files) are rewritten by the files
   phase *without* a causal reload/restart. Is there observed staleness in
   the dogfood profile (samba not reloading after smb.conf changes), or is
   this speculative? Check how the smb step currently handles reloads.
2. **Automatic vs declared** — dotdrift *knows* which managed files belong to
   which unit (it emits both): notify could be emitted automatically for
   smb.conf → smb.service, unit files → daemon-reload. Declared `notify` in
   module.toml adds schema; automatic adds inference. Which?
3. **masked** — is there a real module that wants a unit masked (not just
   stopped+disabled)? One field on `BootstrapService`; cheap, but YAGNI
   without a consumer.
4. **Phase ordering** — mise runs notifications after all files converge;
   dotdrift's mounts/smb steps are separate `--only services` calls. Does
   adopting notify require merging the files+services emission into one
   bootstrap invocation, or does mise order across `--only` phases correctly?
