---
type: Issue
title: Declare services in module.toml instead of systemctl hooks
description: A [services] section in module.toml that converges through mise bootstrap.services, replacing systemctl enable hooks and their optional flag.
tags: [mise, services, systemd, tui]
timestamp: 2026-11-17T00:00:00Z
---

# ISSUE 0159: Declare services in module.toml instead of systemctl hooks

- **Type**: feature
- **Status**: done
- **Priority**: high
- **Labels**: [mise, services, systemd, tui]
- **Assignee**: none
- **Related**: [0048](../archive/README.md), [contract invariant 13](../product/contract.md)
- **Related code**: [internal/profile/](../../internal/profile/), [internal/resolve/](../../internal/resolve/), [internal/mise/](../../internal/mise/), [internal/service/](../../internal/service/), [internal/tui/](../../internal/tui/)
- **Closing commits**: none

## Summary

A module declares the services it needs in a [services] section.
dotdrift converges them through mise [bootstrap.services]. The generic
hook form, systemctl --user enable --now <unit> with optional = true to
swallow the failure, goes away.

## Details

dotdrift already covers the units it writes itself: [systemd.units] in
module.toml becomes [bootstrap.linux.systemd.units] (issue 0048), with
validation, plan, apply, and full TUI editing. Two service shapes have
no declaration today:

1. Enable and start a unit that already exists, installed by a package
   or placed by a dotfile. Today this is a hook:
   systemctl --user enable --now myservice || true, usually marked
   optional so a missing unit does not fail the apply. The failure is
   the point: an optional hook hides a unit that never started.
2. Run a program in the background without writing a unit file.

mise covers both in [bootstrap.services] (checked 2026-11-17 against
mise 2026.11.9 docs). An entry with scope = "user" and no command
enables and starts the existing unit and writes nothing. An entry with
scope = "user" and a command runs the program; mise writes the
platform definition. System entries enable and start units the system
manager provides. State, enabled, restart, environment, and
working_directory are declarative; apply changes only what differs,
and a missing unit is an error with the unit name, not a swallowed
hook failure.

Design:

    [services.my-agent]
    command = "~/.local/bin/my-agent --serve"
    state = "running"
    enabled = true

    [services.syncthing]
    # no command: enable the existing user unit

The module scope decides the service scope, as with the other
sections: a user module declares user services, a system module
declares system services. command is user scope only; mise system
services cannot define a program. restart, environment, and
working_directory require command; mise rejects them on an existing
unit entry, so dotdrift rejects them at resolve with the module named.
A service name that also appears in [systemd.units] is a resolve
error: both would write the same unit definition.

Other mise bootstrap sections were reviewed for the same alignment on
2026-11-17: packages, files, directories, accounts, services, systemd
units, secrets, repos (dotdrift clones the profile itself), compose,
firewall, apt sources, launchd agents, macOS settings, shell
activation, and the login shell. This issue does services only, the
section that replaces hooks in the field. The rest are separate
issues if a user needs them.

## Implementation plan

Core, red first at each layer:

1. profile: [services] in ModuleConfig as a typed spec (command,
   state, enabled as a pointer so absent differs from false, restart,
   environment table, working_directory). FamilyServices encoder that
   writes module.toml back, registered with tomlsplice and the service
   config families.
2. resolve: merge services across layers by name, whole entry wins,
   like systemd units. Validate name charset, state values, restart
   values, the command-only keys, and the systemd.units name conflict
   at plan level. Entries carry module, layer, and the module scope.
3. mise: extend BootstrapService and GenerateBootstrapServices with
   scope, command, state, restart, environment, and working_directory.
   Entries with none of the new fields keep the current inline form,
   so mounts and smb output does not change.
4. service: a services step that writes its own config and runs
   mise bootstrap --only services, after the systemd units step.
   Section name services with the apply section flags. Plan text,
   plan JSON, and drift: user services probe systemctl --user,
   system services reuse the existing service check.
5. cmd: --services and --no-services flags, plan sections.

TUI, after the core compiles:

6. Workspace gains a services section next to systemd.units: one
   container row per service, one row per field, the environment
   table as a value row. Rows edit in place through the structural
   machinery, with pickers for state, restart, and enabled. Add and
   remove a service through the add form and the remove path, same
   as systemd units.
7. The palette lists services as an add target. The plan view counts
   services. The apply gate lists the services section toggle with
   the other sections.

Migration is manual and per module: delete the systemctl hook,
declare the service. No hook text is rewritten in place; a hook is a
command, and dotdrift does not guess which of its words are the unit.

## Acceptance Criteria

- [x] A module with [services] and no command enables an existing unit through mise bootstrap --only services, with scope from the module scope
- [x] A user module with command emits a [bootstrap.services] entry with scope, command, state, enabled, restart, environment, and working_directory
- [x] restart, environment, or working_directory without command is a resolve error naming the module and the service
- [x] A name in both [services] and [systemd.units] is a resolve error
- [x] Mounts and smb [bootstrap.services] output is byte-identical to before
- [x] Plan text, plan JSON, and the TUI plan view show the services section
- [x] The TUI workspace shows services, edits their fields, and adds and removes a service
- [x] The apply gate and the CLI section flags include services
- [x] Status reports a declared service that is not enabled or not active as drift
- [x] go test ./... and go vet ./... pass

## Out of Scope

- Built-in mise services (history-watch, tool-update) and
  requires_tools. No dotdrift module needs them today.
- Other mise bootstrap sections: compose, firewall, apt sources,
  repos, launchd, macOS settings, shell activation, login shell,
  bootstrap.files for system files. Separate issues on demand.
- Rewriting existing hooks automatically.

## Notes

Research basis: the mise bootstrap, systemd, and services pages,
fetched 2026-11-17. mise manages only the unit files it writes
(dev.mise.* for systemd units); deleting a [systemd.units]
declaration leaves the unit installed, and state = "absent" is the
declarative removal. dotdrift [systemd.units] has no absent form
today; that gap is noted here, not fixed in this issue.
