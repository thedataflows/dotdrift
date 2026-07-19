# Directory Update Log

## 2026-07-14

* **Creation**: Initial OKF plan for dotdrift (TDD milestones, mise ensure, defaults-first CLI).
* **Update**: Renamed project forge → dotdrift; apply always resumes; onboard enable-by-presence.
* **M0 complete**: CLI skeleton with all subcommand stubs, `--help` works without `os.Exit` in tests, `go test ./...` green.
* **M1 complete**: Implemented `internal/facts`, `internal/profile`, and `dotdrift modules`. Added fixture profiles under `testdata/profiles` and TOML-based selection with disable-union and `when` filters.
* **M2 complete**: Implemented `internal/resolve`, `dotdrift plan`, host/user overlays, and selection fingerprint.
* **M3 complete**: Implemented `internal/state` and `internal/apply` resume-only pipeline driver.
* **M4 complete**: Implemented `internal/packages` paru/pacman backend.
* **M5/M6 complete**: Implemented `internal/mise` bootstrap, tools/dotfiles steps, and mise config generation.
* **M7 complete**: Implemented `internal/detect` for host, user, os, gpu, and backend facts.
* **M8 complete**: Implemented `internal/onboard` and `dotdrift onboard`.
* **M9 complete**: Hardened `cmd/init` and `cmd/status` with tests, created `README.md` and `examples/profile`, and updated `docs/log.md`. Final `go test ./...` green.
* **M10 complete**: Added `Apt` and `Dnf` backend skeletons with tests, `auto` backend resolution from os-release, and advisory `flock` around state file load/save.
* **Resumed**: Fixed `cmd/root.go` help handling to print `--help` in the real binary while keeping tests `os.Exit`-safe; `main.go` now prints `ExitError` details; cleaned duplicate `/etc/os-release` read in `internal/detect`; implemented `internal/mise` system-wide vs user-managed classification per OKF with `DOTDRIFT_MISE_SYSTEM` override and self-update preference; closed remaining gaps against docs: `init` now creates `modules/`, `hosts/`, `users/`; `onboard` accepts `--profile`; mise bootstrap also checks well-known paths `~/.local/bin/mise` and `~/.local/share/mise/bin/mise`; fixed `.golangci.yml` for v2 schema (`linters.settings` instead of `linters-settings`) and made the project lint-clean.

## 2026-07-15

* **Planned**: Make default state file profile-specific under the XDG state directory so resume state does not collide across profiles; wire `packages.absent` through to the backend `Absent` method so explicit exceptions actually uninstall packages.

* **Implemented**: Default state path is now profile-specific under the XDG state directory (`$XDG_STATE_HOME/dotdrift/profiles/<hash>/state.json`, defaulting to `~/.local/state/dotdrift/profiles/<hash>/state.json`) for `apply` and `status`; `packages.absent` is now propagated through `resolve.PackagesStep.Remove` and executed by the package backend during apply. Verified with `go test ./...`, `go vet ./...`, `golangci-lint run ./...`, and `go build`.

* **Updated**: `docs/tasks/t3-state.md` and `docs/tasks/t4-packages.md` now describe the XDG profile-specific state path and the `packages.absent` uninstall behavior. Added `TestDefaultPath_*`, `TestProfileStatePath_defaultsToLocalState`, and `TestPackagesStep_noPackagesNoBackendCalls`.

## 2026-07-19

* **Added**: `.github/workflows/ci.yml` — CI workflow running `go test ./...` and `go vet ./...` (with `GOFLAGS=-mod=vendor`) plus golangci-lint v2 on push to `main` and pull requests, closing the M0 "CI runs test + vet" exit criterion.
