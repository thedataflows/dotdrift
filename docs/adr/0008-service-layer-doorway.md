# The service layer is one Go package and the contract itself

dotdrift had ~17 deep-module packages under `internal/` and all the
product orchestration — plan assembly, apply step wiring, cursor and
snapshot bookkeeping, lock handling — living in `cmd/`, duplicated per
command and unreachable from anything that is not the kong CLI. A second
front end (`dotdrift tui`) was designed on the same operations. Both
problems have one solution, and a formal IDL was examined first as the
contract for it (issue 0060): protobuf/OpenAPI/JSON Schema/CUE/Smithy
plus a conformance story. Every candidate added a codegen or
mirror-and-check apparatus whose only customer was a non-Go consumer that
does not exist and is out of scope.

We decided: the product surface is **`internal/service` — one Go package
of per-area service structs (reads, writes, session) composed into a
root**, and that Go layer **is** the normative API contract; no IDL is
checked in (issue 0060, revoked-premise resolution). The layer wraps the
domain packages and absorbs the orchestration from `cmd/` (0061-D6):
domain boundaries and public APIs stay untouched, coordination moves in.
Errors cross the boundary typed — `StepError`, `AlreadyRunningError`,
`SessionCancelledError`, `SchemaError` — matched with
`errors.Is`/`errors.As`, never string parsing. The apply session carries
a `Handover(*exec.Cmd)` seam so a step can take the real terminal without
the service importing any UI package. Canonical text renderers live in
the layer only where CLI and TUI must be fact-identical; `--json` stays a
dumb marshal in `cmd/`. `cmd/` migrates onto the layer in slices — reads,
then writes, then the apply session — each green with byte-identical CLI
output, so the big-bang doorway never once leaves the repo red.

Why: two front ends need one place where "what dotdrift does" is true;
wrapped-string errors cannot carry the structure a TUI must render; and
the single-flight lock belongs to the one thing that writes convergence
state, not to each command. Versioning dissolves into ordinary Go package
versioning — the escape hatch is a copy-forward to
`internal/service/v2/` — because in-module consumers already share the
module's release cadence.

Consequence: `cmd/` thins to flag translation plus rendering (the apply
adapter landed at issues 0070/0071); the TUI consumes the same areas with
no private back door. Non-Go UIs are deliberately not served — if one
ever exists, an exchange format is derived from the Go types then (0060).
The absence of an IDL is a recorded decision, not an omission; anyone
looking for `api/` should start at `internal/service` and
[service API](../product/service-api.md). The apply session shipped
first (issues 0069–0071); the reads and writes areas migrate under
milestone M14.

References: issues 0060 (IDL revoked), 0061 (architecture and migration
slices), 0064 (apply session design), 0065 (config area), 0069/0070/0071
(session implementation); [service API](../product/service-api.md);
[package layout](../engineering/package-layout.md); milestone M14.
