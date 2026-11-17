---
type: Issue
title: Per-entry scope for declared services
description: A [services] entry may set scope to override the module scope, and service names may contain @ for template instances.
tags: [mise, services]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0160: Per-entry scope for declared services

- **Type**: feature
- **Status**: done
- **Priority**: medium
- **Labels**: [mise, services]
- **Assignee**: none
- **Related**: [0159](0159-declare-services-instead-of-systemctl-hooks.md)
- **Related code**: [internal/resolve/services.go](../../internal/resolve/services.go), [internal/profile/spec.go](../../internal/profile/spec.go)
- **Closing commits**: none

## Summary

A [services.<name>] entry may set scope = "user" or "system" to
override the module scope for that entry. Service names may contain
@ so template instances such as netbird@main can be declared.

## Details

Issue 0159 derives the service scope from the module scope. Field use
in the reference profile breaks that rule: user modules carry sudo
systemctl hooks for system services (sshd in the network module,
power-profiles-daemon in the system module, a disabled rustdesk
service). Converting those hooks to [services] needs the entry to say
scope = "system" while the module stays user scope. The reverse
(user service from a system module) is not needed today but costs
nothing once the field exists.

Template instances need @ in the name: netbird@main.service. mise
accepts full unit names with @ for system services, and the dotdrift
systemd unit charset already allows it.

Rules: scope is "user" or "system"; absent keeps the module scope.
command requires the effective scope to be user. The generated
[bootstrap.services] entry already carries the resolved scope, so
only resolution and validation change. The TUI gains a scope field
row with a user/system picker, and the encoder writes scope when set.

## Acceptance Criteria

- [x] A user module entry with scope = "system" resolves with system scope; command on it is an error
- [x] A name with @ resolves; the generated config carries it
- [x] An unknown scope value is a resolve error naming the module
- [x] The TUI shows and edits the scope field
- [x] go test ./... and go vet ./... pass

## Out of Scope

- masked and on_change for system services. A separate add if a module needs them.
