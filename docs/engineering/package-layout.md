---
type: Reference
title: Go package layout
description: Module structure for dotdrift.
tags: [engineering, go]
timestamp: 2026-07-14T00:00:00Z
---

# Layout

Each top-level `internal/` directory is a deep module with a small public API and tests against it. `cmd/` only wires commands to these modules.

```
.
├── cmd/                      # Kong CLI wiring; no business logic
│   │                         # (apply is the 0061-D7 adapter: flags -> service
│   │                         # ApplyOpts, session drain, event rendering;
│   │                         # exceptions: cmd/init.go's git orchestration and
│   │                         # cmd/restore.go stay command-local until their
│   │                         # service areas exist; anything richer belongs in
│   │                         # internal/)
├── internal/
│   ├── profile/              # Load dotdrift.toml + modules; selection
│   ├── resolve/              # Merge host/user layers into Plan
│   ├── state/                # Resume cursor persistence
│   ├── apply/                # Pipeline orchestration
│   ├── drift/                # Plan-vs-system drift probes (status)
│   ├── packages/             # Package backend interface + paru/pacman, apt, dnf
│   ├── paru/                  # Embedded mise paru package plugin (go:embed) + lifecycle + `dotdrift paru` subcommand
│   ├── mise/                 # Bootstrap, tools, dotfiles via mise
│   ├── service/              # Service layer: per-area ops (apply session:
│   │                         # run handle, event vocabulary, 0064/0069;
│   │                         # reads areas + canonical renderers, M14).
│   │                         # Writes areas migrate here slice by slice.
│   │                         # Versioning is the module's; breaking changes
│   │                         # copy forward to internal/service/v2 (0061-D1)
│   ├── facts/                # Shared Facts type (hostname/user/os/distro/gpu/backend)
│   ├── detect/               # Host/user/os/gpu facts
│   └── onboard/              # Module factory + copy
├── testdata/                 # Fixture profiles
└── examples/                 # Example profile for users
```

# Rules

- Keep the public API of each package small and stable.
- Use `internal/` freely; callers outside the package must not depend on it.
- Tests live next to code. Prefer fakes for `exec` and filesystem.
- `cmd` never duplicates logic already in an internal package.
