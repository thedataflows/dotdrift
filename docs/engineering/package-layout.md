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
├── internal/
│   ├── profile/              # Load dotdrift.toml + modules; selection
│   ├── resolve/              # Merge host/user layers into Plan
│   ├── state/                # Resume-only state persistence
│   ├── apply/                # Pipeline orchestration
│   ├── packages/             # Package backend interface + paru/pacman
│   ├── mise/                 # Bootstrap, tools, dotfiles via mise
│   ├── detect/               # Host/user/os/gpu facts
│   ├── onboard/              # Module factory + copy
│   └── testutil/             # Fakes, fixtures, golden helpers
├── testdata/                 # Fixture profiles
└── examples/                 # Example profile for users
```

# Rules

- Keep the public API of each package small and stable.
- Use `internal/` freely; callers outside the package must not depend on it.
- Tests live next to code. Prefer fakes for `exec` and filesystem.
- `cmd` never duplicates logic already in an internal package.
