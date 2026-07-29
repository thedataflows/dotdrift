# CONTEXT

Glossary of domain terms for dotdrift. No implementation details — language only.

## Terms

### Layer

Where configuration is declared within a profile, and who it applies to.
Three layers exist, in increasing precedence: **base** (`modules/`, applies
to every host and user), **host** (`hosts/<hostname>/`, applies to one
machine), **user** (`users/<username>/`, applies to one account). Higher
layers override lower layers for the same thing. "Global" means the base
layer.

### Scope

The privilege a module's files are applied with: `user` (as the invoking
user) or `system` (with root privileges). Scope is about *who owns the
target path*, not *who the config applies to* — that is Layer. The two are
independent: a host-layer module can be system-scoped.

### Module

A directory of related configuration plus a `module.toml` describing it.
Presence in any layer means managed; there is no enable list.

### Filter

An explicit, per-invocation narrowing of scope to named modules on `modules`,
`plan`, and `apply`. A filter only ever shrinks what is already selected —
it can exclude, but never resurrect a module that was skipped for its own
reason (disabled, `when` mismatch). Absence of a filter means full scope.

### Generate

The act of producing module content (files and `module.toml`) from
user-supplied choices — interactively or via flags — as opposed to
`onboard`, which adopts existing live files. Generated content lands in a
module at a chosen layer and is then applied like any other module content.

### Mount

A filesystem attachment described declaratively (source, destination,
filesystem type, options, optional schedule). Two kinds exist: **volume
mounts** (local block devices, identified e.g. by UUID) and **network
mounts** (remote exports such as NFS or CIFS).

### Share

A directory exposed to other machines via the Samba server. Shares are
declared independently of mounts — a share's path may coincide with a
mount's destination, but no derivation or coupling exists between the two.

### Placement vs Activation

Putting a config file where the system expects it (placement) is a
different act from making the system use it (activation). Placement is
declarative and diffable; activation is a side effect (reloading a daemon,
enabling a unit, restarting a service). The two are never conflated:
generated content is placed like any other module content, and activation
happens afterwards, idempotently.

### Verbose

An opt-in, per-invocation widening of observability on `apply` and `onboard`
(`--verbose`, `DD_VERBOSE=1`): child-process output (mise operations,
package manager commands, activation commands) streams live to the terminal
instead of being captured and discarded. Absence means quiet — output is
captured and surfaced only on failure. Probes (version checks,
installed-package queries) are never streamed: they exist to be parsed, not
seen.
