---
type: Task
title: T-verbose-output
description: --verbose flag on apply/onboard streaming child-process output live instead of capture-and-discard.
tags: [task, tdd, cli, ux]
timestamp: 2026-07-29T00:00:00Z
---

# Goal

`dotdrift apply` and `dotdrift onboard` gain `--verbose` (default false).
When set, child-process stdout/stderr — mise operations, package manager
commands, mounts/smb activation commands — streams **live** to the terminal
instead of being captured and discarded. When unset, behavior is
byte-identical to before. kong `DefaultEnvars("DD")` makes `DD_VERBOSE=1`
work with no extra code.

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
  `Verbose bool `help:"Stream package manager and mise output live"
  default:"false"``. Apply sets the field on the concrete `*mise.Mise` from
  the `defaultMise` seam and passes the flag to packages/mounts/smb runners
  through the narrow `verboseRunner interface{ SetVerbose(bool) }`
  (`setVerboseRunner`) — interface shapes unchanged, fakes untouched. The
  `newMountsRunner`/`newSmbRunner` seams now return `*ExecRunner` (pointer,
  so `SetVerbose` is reachable). Onboard's real-mise path switched from a
  direct `mise.DefaultMise()` call to the shared `defaultMise` seam so tests
  can observe the constructed value.
- No `--verbose` on other commands; kong `DefaultEnvars("DD")` covers
  `DD_VERBOSE` for free (locked by test).
