# Mount specs live in module.toml as [mounts.\<name\>]

Mount declarations are stored as a keyed `[mounts.<name>]` table inside the
module's own `module.toml`; rendered systemd unit files (and later Samba
share configs) are pure derived artifacts, regenerated on each `generate`
run. Layers merge whole-entry by name: a higher layer's `[mounts.<name>]`
fully replaces the lower layer's entry, matching the existing per-target
dotfile merge semantics.

Why TOML-in-module.toml over a separate mounts.yaml: every other dotdrift
schema is TOML, the module keeps a single source of truth in one diffable
file, layering/override comes for free from the existing merge machinery,
and the generate wizard can preload prior choices by reading the module
back. The pimp-my-cachyos mounts.yaml format is a migration source, not a
format to preserve.

Status: accepted
