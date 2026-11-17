---
type: Issue
title: Services step failure names the declared services
description: When mise bootstrap services fails, the step error lists each declared service with its module and scope, like the packages step does.
tags: [services, mise, errors]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0162: Services step failure names the declared services

- **Type**: bug
- **Status**: done
- **Priority**: medium
- **Labels**: [services, mise]
- **Assignee**: none
- **Related**: [0152](0152-invalid-package-name-names-module-and-config.md), [0159](0159-declare-services-instead-of-systemctl-hooks.md)
- **Related code**: [internal/service/steps.go](../../internal/service/steps.go)
- **Closing commits**: none

## Summary

A failed services step reports "exit status 1" and mise's line. The
error must also list the services in the run with their modules, so
the reader knows which declaration to inspect.

## Details

Field report, 2026-11-17, another device: dotdrift apply boot failed
with "refusing unsafe change to bootstrap service
'limine-snapper-sync'; inspect mise bootstrap plan" and then
"step services: exit status 1". In mise's services code
(crates/mise-system/src/services.rs, mise 2026.11.9) that refusal
fires when the service plan is Unknown: the unit is not found
(LoadState=not-found), the unit exists but cannot be enabled (unit
file state is not disabled, enabled, linked, or indirect, for
example static), or systemctl show failed. Which case applies is a
fact about the device that only the device can give:

    systemctl show limine-snapper-sync.service -p LoadState -p UnitFileState -p ActiveState

The packages step already solves the same shape (issue 0152): its
error lists each package with the declaring modules. The services
step wraps its bootstrap error the same way.

## Acceptance Criteria

- [x] A failing services step error lists each declared service with its module and scope
- [x] go test ./... and go vet ./... pass

## Out of Scope

- Changing which services mise refuses. That is mise's safety call.
- A per-entry optional flag for services that may be absent. That is
  a design question for a device that proves it needs one.
