---
okf_version: "0.1"
---

# dotdrift implementation plan

Progressive disclosure for agents implementing [dotdrift](product/contract.md).

# Product

* [Product contract](product/contract.md) - Invariants the CLI must never violate
* [CLI surface](product/cli-surface.md) - Commands and defaults
* [Profile layout](product/profile-layout.md) - Git-backed profile structure
* [Merge rules](product/merge-rules.md) - Host vs user precedence
* [Mise bootstrap](product/mise-bootstrap.md) - Detect, min version, upgrade policy
* [Migrate from pimp-my-cachyos](product/migrate-pimp-my-cachyos.md) - Legacy mounts/shares migration recipe

# Engineering

* [TDD rules](engineering/tdd-rules.md) - Binding process for every task
* [Package layout](engineering/package-layout.md) - Go module structure
* [Definition of done](engineering/definition-of-done.md) - v0.1.0 exit criteria

# Milestones

* [Milestones](milestones/) - Ordered delivery slices M0–M10

# Tasks

* [Tasks](tasks/) - TDD task specs

# Issues

* [Issues](issues/) - Discovered work (bugs found via dogfooding/e2e, emergent features, chores); distinct from [milestones](milestones/) and [tasks](tasks/) which cover planned work