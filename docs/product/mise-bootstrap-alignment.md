---
type: Research
title: Mise bootstrap feature alignment
description: Which mise capabilities dotdrift adopts, adopts in part, or does not adopt, and why. Current as of mise 2026.11.9 (2026-11-17).
tags: [product, mise, bootstrap, research]
timestamp: 2026-11-17T00:00:00Z
---

# Mise feature alignment

Basis: the mise documentation and changelog at mise 2026.11.9
(2026-11-16), fetched 2026-11-17, and the mise 2026.11.8 binary on
this machine. dotdrift requires that floor family: MinMiseVersion is
2026.11.9 (issue 0158). This document is the map. Adoption is one
issue per feature; this document does no work itself.

## Adopted

| mise capability | dotdrift form | Since |
|---|---|---|
| [tools] | module.toml [tools], one conf.d snippet per module | base design |
| [bootstrap.packages] with manager plugins | module.toml [packages], the embedded paru plugin on Arch | 0002, 0003, 0054 |
| [bootstrap.users] and [bootstrap.groups] | smb group and users | 0040 |
| [bootstrap.directories] | mount destinations and missing dotfile parents that do not exist yet | 0151, 0154 |
| [bootstrap.secrets] with secret() in templates | module.toml [secrets], system dotfiles config | 0044 |
| [bootstrap.services] for mount and smb units | generated from [mounts] and [smb] | 0002 |
| [bootstrap.linux.systemd.units] | module.toml [systemd.units], passthrough directives, user scope only | 0048 |
| [dotfiles] modes symlink, symlink-each, copy, template | module.toml [dotfiles] whole-file entries, mode kept verbatim | 0002, 0156 |
| [dotfiles] line, block, and template edits | module.toml edit entries, both scopes | 0042 family, 0156 |
| dotfiles depends (mise 2026.11.0) | computed from source and target paths; chained entries converge in one apply | 0158 |
| min_version in mise.toml | written into every generated config | 0158 |
| [bootstrap.services] declared services | module.toml [services]: an entry without command enables an existing unit; command runs a program (user scope) | 0159, 0160 |
| systemd service state from declarations | services step runs after the systemd step; mise runs one daemon-reload before system service changes | 0159 |

## Adopted in part

| mise capability | What dotdrift takes | What it does not take, and why |
|---|---|---|
| [bootstrap.files] | The section survives only as a probe source for drift and diff, and [bootstrap.directories] above | Whole-file system entries moved back to [dotfiles] in 0156: [bootstrap.files] has no symlink form (mode is an octal string), so a declared symlink landed as a copy. The September version of this map recommended [bootstrap.files] (A3, done in 0042); 0156 reversed that for whole-file entries. Block edits in [bootstrap.files] (mise 2026.11.8), with owner, group, mode, phase, and notify, are the standing spike: they cover block edits only, line entries have no form there |
| [bootstrap.services] options | state, enabled, scope (module, overridable per entry), command, restart, environment, working_directory | masked and on_change. on_change fires only when a [bootstrap.files] entry lists the service in notify, and dotdrift system files are [dotfiles] entries, which cannot notify. Restart on a changed file therefore stays a post hook (sshd, systemd-logind, snapperd in the reference profile) until a file moves to [bootstrap.files]. Built-in services (history-watch, tool-update) and requires_tools: no module needs them |
| Dotfiles entry keys | source, mode, line, block, comment, template, and the computed depends | content (inline whole file), permissions-only entries, create_if_missing, relative symlinks, dot_prefix, exclude and manifest on directory sources, variants. Each is a small module.toml addition on user demand; none has a user today. Track and track-local modes are rejected, not unimplemented: dotdrift keeps sources in modules and has its own drift |
| Dotfiles history | Nothing | dotdrift status and orphans already answer drift; a mise history store would be a second copy of the same files |
| Task features used by hooks | One task per hook command, module-namespaced names, interactive = true | Task locks, lock pools, tag-selected dependencies, --report-file, and the Unix socket sandbox. Hooks run one at a time; the sandbox would break hooks that reach the network or a package manager. --report-file is the only one with a visible gain (structured hook failure data) and waits for a need |

## Not adopted

| mise capability | Reason |
|---|---|
| [dotfile_groups] | The module is dotdrift's group: one directory, layered, with its own sources. Two grouping systems for the same files would conflict |
| [bootstrap.repos], --adopt, --from-git | dotdrift clones and updates the profile repository itself; module companion checkouts have no user today |
| [bootstrap.compose] | No module declares a compose project. Large surface, separate issue on demand |
| [bootstrap.linux.firewall] | Host-critical and no profile declares firewall state. Separate issue on demand |
| [bootstrap.apt.sources], dnf repos, and the other package managers (nix, flatpak, snap, zypper, apk, brew, scoop, winget, choco, mas, macos-app) | dotdrift prefixes through the detected backend (paru, apt, dnf family). Manager-specific repository sections are an addition per backend on demand |
| [bootstrap.macos.launchd.agents], [bootstrap.settings.macos], bootstrap settings plugins | dotdrift targets Linux profiles today |
| [bootstrap.mise_shell_activate] and [bootstrap.user] login shell | Shell activation lives in the shell module's dotfiles, where the user edits it. A generated block would fight the edit entries. Separate issue if a user wants mise to own it |
| [bootstrap.remote] and mise ssh | dotdrift runs one profile locally per machine; hosts and users layers are the multi-machine answer |
| Daemons (experimental) | Development services for a project. dotdrift manages machine services through bootstrap services and systemd units |
| mise.lock pins for [tools] | A machine profile wants the declared version on each machine. Checksums add a step to every version change. Revisit if a bad tool release breaks a machine |
| Tool provenance, minimum_release_age, packslip verification | Inherited free when mise verifies on its own side; dotdrift adds no policy of its own today |

## History of this map

The September 2026 findings, with their outcomes:

- A1, [bootstrap.users] primary group: done, 0040.
- A2, pin the working directory for mise calls: done, 0041.
- A3, system files through [bootstrap.files]: done in 0042,
  reversed for whole-file entries in 0156 (no symlink form). See
  Adopted in part above before acting on the old text.
- A4, built-in aur and pacman managers: reverted in 0054. Bare Arch
  names stay paru: and the embedded plugin stays; the aur/ marker
  switch waits on 0045.
- A5, package version pins: open. A module.toml schema addition,
  not a translation change.
- Secrets: done, 0044. systemd user units: done, 0048.

## Standing candidates, in order

1. [bootstrap.files] for system block edits, with owner, group,
   mode, phase, and notify (unblocks on_change for the logind and
   snapper files). Spike on one module first.
2. masked and on_change on [services] system entries: tracked as
   [issue 0050](../issues/0050-service-notify-on-change.md) since
   September, still open. Its blocker in November terms: no file
   dotdrift emits can notify, because system files are [dotfiles]
   entries. Candidate 1 creates the first notifying files.
3. Merge edits in module.toml (mise merge and merge = "missing"
   for JSON, JSONC, TOML, YAML, INI): the one clear gap in the
   dotfile edit forms, for files applications rewrite.
4. Package version pins for apt and dnf (A5): tracked as
   [issue 0049](../issues/0049-package-version-pins.md), still open.
5. state = "absent" for [systemd.units]: deleting a declaration
   leaves the unit installed today.
6. Login shell and shell activation: tracked as
   [issue 0051](../issues/0051-login-shell-and-activation.md), still
   open. The shell module's edit entries cover it today.
