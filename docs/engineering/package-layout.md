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
│   │                         # (exception: cmd/init.go's git clone/create
│   │                         # orchestration is intentional command wiring,
│   │                         # not domain logic; anything richer belongs in internal/)
├── internal/
│   ├── profile/              # Load dotdrift.toml + modules; selection
│   ├── resolve/              # Merge host/user layers into Plan
│   ├── state/                # Resume-only state persistence
│   ├── apply/                # Pipeline orchestration
│   ├── packages/             # Package backend interface + paru/pacman, apt, dnf
│   ├── mise/                 # Bootstrap, tools, dotfiles via mise
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
