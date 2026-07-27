---
type: Guide
title: Migrate from pimp-my-cachyos
description: Move mounts and Samba shares from the legacy pimp-my-cachyos mise scripts to dotdrift generate.
tags: [product, migration, generate]
timestamp: 2026-07-28T00:00:00Z
---

# Migrate from pimp-my-cachyos

How to move the mounts and Samba shares managed by the legacy
`~/pimp-my-cachyos` setup onto `dotdrift generate mounts|smb`. The
dotdrift registry presets derive from the legacy script, so rendered
units carry the same option sets — this is a format migration, not a
behavior change (see [ADR-0002](/adr/0002-mount-specs-in-module-toml.md)).

## What the legacy files did

- `mountpoints/mounts.yaml` — global mounts, merged with
  `mountpoints/mounts.<hostname>.yaml` (per-host mounts) via `yq
  explode(.)`. Each entry: `source`, `destination`, `type`, optional
  `options`, `startat`, `state`, `share`.
- `mise-tasks/system/mountpoints.sh` — read the merged YAML, picked an
  option preset per filesystem type (kernel-gated ntfs3 vs ntfs-3g),
  rendered `.mount`/`.service`/`.timer` units through `envsubst`
  templates into `/usr/local/lib/systemd/system/`, created the
  destination directories, ran `daemon-reload`, and `enable --now`'d
  each unit (or `disable` for `state: disabled`). Unit names were the
  destination path with slashes turned to dashes — the same names
  dotdrift's `EscapePath` produces.
- `mise-tasks/system/smb.sh` — installed samba/avahi, copied a static
  `smb.conf` to `/etc/samba/`, created the `smb` group, added the user,
  ran `smbpasswd -a`, wrote one `/etc/samba/smb.conf.d/<name>.conf` per
  share, appended `include = ...` lines to `smb.conf`, and
  enabled/restarted the `smb` service. Shares came from
  `mounts.<hostname>.yaml` only: every entry **without** `share: false`
  was shared.

## Step 0: create or select a profile

```bash
dotdrift init ./my-profile && cd ./my-profile
```

## Step 1: dump the legacy entries

A `yq` one-liner that flattens the legacy YAML into
name/source/destination/type/startat/options rows, as a starting point
for the invocations below:

```bash
yq -r 'to_entries[] | [.key, .value.source, .value.destination, .value.type, (.value.startat // ""), (.value.options // "")] | @tsv' \
  ~/pimp-my-cachyos/mountpoints/mounts.yaml \
  ~/pimp-my-cachyos/mountpoints/mounts."$(hostname)".yaml
```

## Step 2: translate mounts

Each legacy entry maps to one `dotdrift generate mounts` invocation.
Flags map 1:1 to the legacy keys; when `--option` is omitted entirely,
the registry preset for `--type` applies (the same presets the legacy
script hard-coded).

**Important:** one CLI run writes one mount, and `[mounts]` is replaced
wholesale in the target module. Two working patterns:

- **Scripted — one module per mount** (distinct `--module`; each module
  is idempotent and independently rerunnable). This is the pattern used
  below.
- **Interactive — one module, many mounts**: run `dotdrift generate
  mounts` in a terminal and use the wizard's "add another mount?" loop;
  it accumulates all mounts and writes once.

### Global mounts (base layer)

The three NFS mounts from `mounts.yaml`, all with `startat *-*-*
18:05:00`:

```bash
dotdrift generate mounts --module synology-syn01 --name synology-syn01 \
  --source synology.local:/volume1/syn01 --destination /mnt/synology/syn01 \
  --type nfs --startat "*-*-* 18:05:00"

dotdrift generate mounts --module synology-syn02 --name synology-syn02 \
  --source synology.local:/volume2/syn02 --destination /mnt/synology/syn02 \
  --type nfs --startat "*-*-* 18:05:00"

dotdrift generate mounts --module synology-shared --name synology-shared \
  --source synology.local:/volume2/shared --destination /mnt/synology/shared \
  --type nfs --startat "*-*-* 18:05:00"
```

### Per-host mounts (`--layer host --hostname <name>`)

Legacy `mounts.<hostname>.yaml` files map to the host layer
([ADR-0001](/adr/0001-discover-overlay-only-modules.md) — a module
existing only under `hosts/<hostname>/modules/` is selected like a base
module; no base stub needed). From `mounts.cri-pc.yaml`:

```bash
# win_c: legacy "ntfs" splits by kernel — ntfs3 (kernel >= 7.1) or
# ntfs-3g (kernel < 7.1). Pick per host; the wizard preselects the
# kernel-recommended one. Note: the legacy kernel >= 7.1 path kept the
# bare "ntfs" type with its own preset (umask=027,nocase,preallocated_size=
# 131072); dotdrift's registry has no bare "ntfs" entry, so that exact
# preset is not carried over — ntfs3's preset matches the reference's
# ntfs3 case verbatim. The share:false key is dropped (see step 3).
dotdrift generate mounts --layer host --hostname cri-pc --module win-c --name win_c \
  --source UUID=01D97D07D7C008F0 --destination /mnt/win/c --type ntfs3

# linux1-4: plain btrfs, registry preset (auto,rw,relatime) applies
dotdrift generate mounts --layer host --hostname cri-pc --module linux1 --name linux1 \
  --source UUID=2e768ab4-1e25-40c9-934e-b9e77af557ba --destination /mnt/linux1 --type btrfs
# ... same shape for linux2 (486939e7-...), linux3 (b79b7dd2-...), linux4 (9d589834-...)

# ventoy: custom options. Any --option overrides the whole preset (no
# merge); bare uid/gid tokens expand to the invoking user's numeric ids,
# replacing the legacy literal uid=cri/gid=cri.
dotdrift generate mounts --layer host --hostname cri-pc --module ventoy --name ventoy \
  --source UUID=7FFD-B6CF --destination /mnt/ventoy --type exfat \
  --option auto --option rw --option uid --option gid --option noatime
```

And `mounts.cri-laptop.yaml` translates the same way with `--hostname
cri-laptop`. Legacy `state: disabled` (commented out on `win_c`) maps to
`--state disabled`.

## Step 3: re-declare the shares

The legacy `share:` key is **dropped** — shares are declared
independently of mounts (the CONTEXT glossary: "no derivation or
coupling exists between the two"). The legacy script shared every
host-yaml entry without `share: false`, so on cri-pc that was linux1-4
**and ventoy** (no `share` key meant shared); `win_c` was excluded by
its explicit `share: false`.

`--share` is repeatable, so one `generate smb` run declares them all
(group `smb`, user `cri`, writable by default, avahi on by default):

```bash
dotdrift generate smb --layer host --hostname cri-pc \
  --share linux1=/mnt/linux1 --share linux2=/mnt/linux2 \
  --share linux3=/mnt/linux3 --share linux4=/mnt/linux4 \
  --group smb --user cri
```

Add `--share ventoy=/mnt/ventoy` to reproduce the legacy default
exactly, or leave it out as a deliberate change. `--no-avahi` records an
explicit `avahi = false` and drops the `avahi` package.

## Step 4: apply

```bash
dotdrift plan     # review the mounts:/smb: sections
dotdrift apply
```

On apply, placement and activation stay separate
([contract](/product/contract.md) invariant 14):

1. **packages** — the backend installs the union: registry packages per
   mount type (`nfs-utils`, `nfsidmap`, `ntfsprogs-plus`, …) plus
   `samba` and `avahi`.
2. **dotfiles-system** — mise copies the generated units to
   `/usr/local/lib/systemd/system/` and `smb.conf` + `shares.conf` into
   `/etc/samba/` (`smb.conf` is seeded once in the module and then
   user-owned; edit the module copy, not `/etc/samba/smb.conf`).
3. **mounts step** — `mkdir -p` destinations, one `daemon-reload`, then
   `enable --now` per `.mount` (plus `.timer` for `startat` entries);
   `state = "disabled"` entries get a tolerant `disable --now`. Note the
   deliberate strictness delta: the legacy script swallowed
   `enable --now` failures (`|| true`); dotdrift fails the step loudly
   (resume re-runs it), so a broken unit surfaces instead of hiding.
4. **smb step** — creates the group and memberships, enables
   `avahi-daemon` when avahi is on, gates on `testparm -s` (a failure
   stops before any restart), enables/restarts `smb`, and runs
   `smbpasswd -a <user>` interactively on a terminal for each declared
   user missing from `pdbedit -L`.

## Retirement checklist

The legacy script wrote units to the **same paths** dotdrift now manages
(`/usr/local/lib/systemd/system/<escaped-destination>.{mount,service,timer}`),
so retire the old management before the first apply to avoid duplicate
unit ownership:

1. **Do this before `dotdrift apply`:**
   - Disable and stop the old units written by the legacy script, e.g.
     `sudo systemctl disable --now mnt-synology-syn01.mount
     mnt-synology-syn01.timer` (and the same for every migrated mount).
     dotdrift's mounts step re-enables them after mise places the new
     unit content.
   - For legacy mounts you are **not** migrating, also remove the old
     unit files from `/usr/local/lib/systemd/system/` — dotdrift only
     garbage-collects stale units inside its own modules, not files the
     old script wrote.
   - Remove the old per-share files `/etc/samba/smb.conf.d/<name>.conf`
     written by `smb.sh`. The legacy `include = ...` lines appended to
     `/etc/samba/smb.conf` disappear when mise places dotdrift's seeded
     `smb.conf` (which statically includes
     `/etc/samba/smb.conf.d/shares.conf`).
2. **Disable the old mise tasks** so they never run again:
   `mountpoints.sh` (alias `sm`) and `smb.sh` in
   `~/pimp-my-cachyos/mise-tasks/system/` (remove them from the mise
   task set or delete the scripts; keep the YAML files as read-only
   reference until the migration is verified).
3. **Verify after apply:** `systemctl list-units 'mnt-*'` shows the
   dotdrift-managed units active, `testparm -s` is clean, and a second
   `dotdrift apply` is a no-op resume.

See [cli-surface](/product/cli-surface.md#generate) for the full flag
reference and [profile-layout](/product/profile-layout.md) for the
`[mounts]`/`[smb]` schemas.
