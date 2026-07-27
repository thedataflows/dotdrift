# Discover overlay-only modules

Module discovery previously scanned only the base layer (`modules/`); host
and user overlays were merged only onto base-selected modules. We decided
that presence = managed holds per layer: a module existing only under
`hosts/<hostname>/modules/` or `users/<username>/modules/` is discovered
and selected like a base module, with absent layers treated as empty during
merge.

Why: the `generate` command lets users choose which layer generated content
lands in, and host-scoped content (e.g. machine-specific mounts identified
by disk UUID) must not require a meaningless base stub to become visible.
This also repairs a latent dead path: `onboard --host` already wrote
host-layer-only modules that resolve never selected.

Consequence: any pre-existing `hosts/*/modules/<id>` directory in the wild
becomes live configuration where it was previously inert. This is a
contract-level behavioral change to invariant 1 (presence = managed).
