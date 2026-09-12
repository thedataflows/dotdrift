# Generate is CLI-only; `tui` is the one interactive home

Contract 15 pinned the `dotdrift generate` interactive wizard and the
flag-driven CLI to byte-equivalent assembly, which was honest only while
both existed. The TUI design map (issue 0057) and the editor suite design
(issue 0065, decisions D6/D7/D9) make `dotdrift tui` the single
interactive surface for mounts/smb. Keeping the wizard anyway would have
meant keeping the huh front-end, the mode-selection machinery
(`selectGenerateMode`, the `isTTY` probe, `runWizard`, the
`wizardInvocation` seam), and a `generate → tui` import pointing backwards
against the four-layer architecture the editor suite builds on.

We decided: `generate mounts|smb` is strict flag mode. The wizard and the
`--tui`/`--no-tui` flags are deleted; kong's own unknown-flag error is the
whole removal UX, with no deprecated stub. The no-flags-without-terminal
actionable error dissolves into ordinary required-flag validation
(`validate()` already named every missing flag). The pure spec-builder
machines and the shared assembly builders (`MountsWizard`/`SmbWizard`,
`KindForDefaults`/`PrefillForVolume`/`ShareLoopDone`, `MountsInput`/
`SmbInput`, `ParseShareFlags`, `ResolveWritable`, `InvokingUser`,
`ExistingMountSources`, `PrintSummary`) move to `internal/generate`, so
`cmd` never imports `internal/tui` and the only arrow between the layers
is the `tui → generate` one the editor suite consumes (0065-D7).
Contract 15 is repointed in place at the prefill seam — the input an
editor prefills equals the input `generate` assembles — because that is
the invariant the surviving equivalence harness already enforces.

Why: one interactive home, one assembly path, and a dependency graph
whose arrows all point the same way. The wizard's state machine was
always the reusable half; the huh forms were the disposable half.

Consequence: muscle memory for `--tui` gets an unknown-flag error —
README and `docs/log.md` carry the announcement instead of a shim.
There is an interim gap with no interactive mounts/smb path, accepted
consciously (issue 0066, D5): the flags assemble everything the wizard
did, and the editor suite is sequenced next. `charmbracelet/huh` leaves
go.mod and the vendor tree; `internal/tui` keeps `theme.go` as the style
registry the future shell reuses (ADR-0003). The seven equivalence tests
survive untouched — they drive the machines against the CLI assembly —
and retarget at editor prefill when the editors land (0065-D9), which is
exactly the new contract 15.

References: issue 0057 (TUI design map), issue 0065 (editor suite, D6/D7/
D9), issue 0066 (this decision), milestone M13 (Generate — the wizard was
its exit criterion; history stands), contract invariant 15 (old and new
text).
