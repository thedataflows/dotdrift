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
