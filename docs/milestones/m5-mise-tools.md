---
type: Milestone
title: M5 Mise tools
description: Ensure mise, generate tools config, mise install.
tags: [milestone]
timestamp: 2026-07-14T00:00:00Z
order: 5
---

# Goal

Tools step uses generated mise.toml; [EnsureMise](/product/mise-bootstrap.md) runs first.

# Exit criteria

- EnsureMise all branches unit-tested ([T-mise-ensure](/tasks/t-mise-ensure.md))
- Tools step + fake runner green

# Tasks

1. [Ensure mise](/tasks/t-mise-ensure.md) (if not done earlier)
2. [T5 mise tools](/tasks/t5-mise-tools.md)

# Depends on

[M3](m3-state-apply.md)