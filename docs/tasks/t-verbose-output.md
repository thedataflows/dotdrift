---
type: Task
title: T-verbose-output
description: --verbose flag on apply/onboard streaming child-process output live instead of capture-and-discard, with -v short alias and set -x-style command echo.
tags: [task, tdd, cli, ux]
timestamp: 2026-07-29T00:00:00Z
---

# Goal

`dotdrift apply` and `dotdrift onboard` gain `--verbose` (default false;
short alias `-v` via kong `short:"v"`). When set, child-process
stdout/stderr — mise operations, package manager commands, mounts/smb
activation commands — streams **live** to the terminal instead of being
captured and discarded, and each command line is echoed `set -x`-style
(`+ <argv>`) to the Err writer (stderr, like bash) immediately before the
command runs. When unset, behavior is byte-identical to before (no stream,
no echo). kong `DefaultEnvars("DD")` makes `DD_VERBOSE=1` work with no
extra code. `-v` collides with nothing: no other short flags exist and
`version` is a subcommand, not a flag.

# Tests first

- `TestExecMise_verboseStreamsOperationOutput` — all four operation methods
  (`EnsureAndInstall`, `DotfilesApply`, `DotfilesApplySudo` via `geteuid`
  seam as root, `RunTask`) stream child stdout/stderr to the injected
  writers against a fake `mise` script on the real exec path.
- `TestExecMise_nonVerboseDiscardsSuccessOutput` /
  `TestExecMise_nonVerboseErrorAppendsCapturedOutput` — today's contract:
  success output discarded, failure output appended to the error, writers
  untouched.
- `TestExecMise_verboseErrorReturnsBareErr` — streaming failure returns the
  bare err (output already visible), nothing appended.
- `TestMise_versionProbeStaysCapturedWhenVerbose` — `mise --version` never
  streams (its output is parsed).
- `TestExecRunner_RunStream_verboseStreamsToWriters` /
  `_nonVerboseBehavesLikeRun` — real `sh -c` echo; non-verbose is
  byte-identical to `Run`.
- `TestBackend_presentAbsentUseStreamingPath` — Paru/Apt/Dnf table:
  install/remove route through the streaming surface when the runner offers
  one (`apt-get update` included).
- `TestBackend_isInstalledStaysOnCapturedPath` — probes (`pacman -Q`,
  `dpkg -l`, `rpm -q`) stay on the captured `Run` path even when the runner
  can stream.
- `TestBackend_setVerbose` — flips the real `ExecRunner` inside a default
  backend; an injected fake runner is left untouched.
- mounts `TestExecRunner_verboseStreamsAndPreservesErrorOutput` /
  `_nonVerboseSuccessDiscardsOutput` / `_nonVerboseErrorAppendsOutput` /
  `_setVerbose` — verbose streams live AND keeps the error's output suffix
  (MultiWriter).
- smb `TestExecRunner_verboseStreamsAndCaptures` /
  `_nonVerboseCapturesOnly` / `_setVerbose` — verbose streams live AND
  returns the combined capture (parsing contract: `id -Gn`, `pdbedit -L`,
  testparm gate).
- `TestApply_verbosePropagatesToRunners` / `_nonVerboseLeavesRunnersQuiet` —
  observed via the existing seams: captured `*mise.Mise.Verbose` field plus
  `SetVerbose`-recording fakes for packages/mounts/smb.
- `TestOnboard_verbosePropagation` — onboard's real-mise path (through the
  `defaultMise` seam) mirrors the flag.
- `TestKong_verboseFlagParses` / `TestKong_verboseDefaultEnvar` — flag
  parses on apply+onboard; `DD_VERBOSE=1` enables it via
  `DefaultEnvars("DD")`.
- executil `TestShellJoin_*` (plain args unquoted, whitespace/tab
  single-quoted, `'` escaped `'\''`, empty argv) +
  `TestEchoCommand_setXStyleLine` — the shared echo helper.
- `TestExecMise_verboseEchoesCommandLine` (all four ops: `+ <argv>` on Err
  BEFORE the command's own stderr output; never on Out) /
  `TestExecMise_nonVerboseNoEcho` /
  `TestMise_versionProbeNeverEchoedWhenVerbose` — echo contract on the mise
  path, probes unechoed.
- packages `TestExecRunner_RunStream_verboseEchoesCommandLine` /
  `TestExecRunner_Run_neverEchoesEvenWhenVerbose` — streaming path echoes,
  captured probe path never does.
- mounts + smb `TestExecRunner_verboseEchoesCommandLine` — verbose echoes
  before streaming; smb additionally asserts the echo never enters the
  returned capture (parsing contract).
- `TestKong_verboseShortFlagParses` — `-v` aliases `--verbose` on apply and
  onboard.

# Implementation notes

- **mise** (`internal/mise/mise.go`): `Mise` gains `Verbose bool` +
  `Out`/`Err io.Writer` (nil → os.Stdout/os.Stderr). New `runOp` streams via
  `cmd.Stdout`/`cmd.Stderr` + `cmd.Run()` when Verbose on the real exec path
  (fakes bypass); otherwise delegates to `runWithEnv`. Only the four
  `ExecMise` operation calls use `runOp`; `ensure()`/`version()` probes and
  the `defaultInstall` bootstrap keep the captured `runContextEnv` path.
  Streaming error → bare err; captured error → output appended as today.
- **packages** (`internal/packages`): verbose lives on `ExecRunner`
  (`Verbose`/`Out`/`Err` + `RunStream`). Backends route `Present`/`Absent`
  through `runMutating`, which type-asserts the runner to the narrow
  `streamRunner` surface and falls back to `Run` — `IsInstalled` probes call
  `Runner.Run` directly and stay quiet. `SetVerbose(bool)` on
  `*Paru`/`*Apt`/`*Dnf` flips the inner real `ExecRunner` only (decision:
  verbose on the runner, backend chooses per method).
- **mounts + smb** (`internal/mounts/mounts.go`, `internal/smb/smb.go`):
  `ExecRunner` gains `Verbose`/`Out`/`Err` + pointer `SetVerbose`. Verbose
  mode wires `io.MultiWriter(live, &buf)` so output streams AND is captured
  (decision: MultiWriter, not a captured exception) — mounts errors keep
  their `argv: err: output` shape and smb's `id -Gn`/`pdbedit -L` parsing
  plus the testparm gate's error output survive verbatim. Non-verbose wiring
  is mechanically identical to `CombinedOutput()`. The interactive
  `smbpasswd` path already streams — untouched.
- **cmd wiring**: `ApplyCmd`/`OnboardCmd` gain
  `Verbose bool `help:"…" short:"v" default:"false"``. Apply sets the field
  on the concrete `*mise.Mise` from the `defaultMise` seam and passes the
  flag to packages/mounts/smb runners through the narrow
  `verboseRunner interface{ SetVerbose(bool) }` (`setVerboseRunner`) —
  interface shapes unchanged, fakes untouched. The
  `newMountsRunner`/`newSmbRunner` seams now return `*ExecRunner` (pointer,
  so `SetVerbose` is reachable). Onboard's real-mise path switched from a
  direct `mise.DefaultMise()` call to the shared `defaultMise` seam so tests
  can observe the constructed value.
- No `--verbose` on other commands; kong `DefaultEnvars("DD")` covers
  `DD_VERBOSE` for free (locked by test).
- **`-v` short alias**: kong `short:"v"` on the same `Verbose` field of both
  commands. Collision check: no other `short:` tags exist anywhere, and
  `version` is a subcommand rather than a flag, so `-v` is unambiguous.
- **Command echo (`set -x` style)**: one shared helper package
  `internal/executil` — `ShellJoin(argv)` (join with spaces; single-quote
  any arg containing whitespace or a single quote, escaping `'` as `'\''`;
  simple args unquoted) and `EchoCommand(w, argv)` (writes `+ <argv>\n`).
  All four streaming call sites use it: mise `runOp`, packages `RunStream`,
  mounts and smb `ExecRunner` verbose paths — the echo is written to the
  same Err writer the runner streams to (stderr, like bash `set -x`; stdout
  stays parseable) immediately before `cmd.Run()`. Captured paths never
  echo: probes (`mise --version`, `pacman -Q`/`dpkg -l`/`rpm -q`, mise.run
  bootstrap) stay silent AND unechoed, and the smb/mounts capture buffers
  never contain the echo line.
