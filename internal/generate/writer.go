package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Layer-aware, idempotent module writer.
//
// WriteModule turns a resolved mount/smb selection into a self-contained
// module directory at one of the three profile layers: rendered systemd
// units and samba configs on disk, and a module.toml whose scope, mounts,
// smb, packages, and dotfile sections describe them so the regular
// dotfiles machinery (mise) places the files and the mounts/smb steps
// activate them.
//
// Idempotency contract: the same input always produces a byte-identical
// module tree (all map iteration and encoding is key-sorted; BurntSushi
// encodes maps sorted by key, verified empirically). Previously generated
// unit files that no longer correspond to an input mount are garbage
// collected; smb.conf is seeded once and then user-owned.
//
// Note: module.toml is decoded into profile.ModuleConfig and re-encoded,
// so comments from a previous hand edit of module.toml are not preserved.

// Layer names for Selection.Layer.
const (
	// LayerBase writes to <root>/modules/<id>.
	LayerBase = "base"
	// LayerHost writes to <root>/hosts/<hostname>/modules/<id>.
	LayerHost = "host"
	// LayerUser writes to <root>/users/<username>/modules/<id>.
	LayerUser = "user"
)

// Placement constants for generated artifacts and their dotfile targets.
const (
	moduleTomlName  = "module.toml"
	sharesFileName  = "shares.conf"
	smbConfFileName = "smb.conf"

	// unitTargetDir is where the generated units are installed. This is
	// /usr/local/lib/systemd/system by design — locally generated units do
	// not belong in /etc/systemd/system.
	unitTargetDir  = "/usr/local/lib/systemd/system"
	sharesTarget   = "/etc/samba/smb.conf.d/shares.conf"
	smbConfTarget  = "/etc/samba/smb.conf"
	dotfileMode    = "copy"
	sambaPackage   = "samba"
	avahiPackage   = "avahi"
	filePermission = 0o644
	dirPermission  = 0o755
)

// unitSuffixes is the naming scheme of generated unit files. Together with
// a dotfile entry pointing into unitTargetDir it defines "generated" for
// garbage collection: a file is collected only when it matches a unit
// suffix AND is tracked in module.toml with a target under unitTargetDir.
var unitSuffixes = []string{".mount", ".service", ".timer"}

// Selection names the module and the layer to write it at.
type Selection struct {
	// Layer is LayerBase, LayerHost, or LayerUser.
	Layer string
	// Hostname is required for LayerHost (the hosts/<hostname> directory).
	Hostname string
	// Username is required for LayerUser (the users/<username> directory).
	Username string
	// ModuleID is the module directory name.
	ModuleID string
}

// Input carries the resolved specs and the uid/gid used for mount-option
// token expansion (the caller resolves them via os/user).
type Input struct {
	// Mounts replaces the module's [mounts] section wholesale when
	// non-nil; nil means mounts are not managed by this run (existing
	// mounts, unit files, and their dotfile entries are left untouched).
	Mounts map[string]profile.MountSpec
	// Smb, when non-nil, replaces the module's [smb] section wholesale and
	// (re)generates shares.conf. Nil means "smb is not managed by this
	// run": an existing smb section, shares.conf, and smb.conf are all
	// left untouched.
	Smb *profile.SmbSpec
	// UID and GID expand the bare "uid"/"gid" option tokens.
	UID int
	GID int
}

// smbConfSeed is the smb.conf written once when a module first gains smb.
// Modeled on the pimp-my-cachyos system-samba reference, reduced to a
// minimal but functional [global] plus the static include of the generated
// shares.conf. After seeding the file is user-owned: WriteModule never
// touches it again.
const smbConfSeed = `[global]
server role = standalone
security = user
workgroup = WORKGROUP
load printers = no
printcap name = /dev/null
disable spoolss = yes
include = /etc/samba/smb.conf.d/shares.conf
`

// WriteModule materializes the module selected by sel under root from
// input. It validates the selection and every mount's filesystem type
// against the registry before writing anything, so an invalid input never
// leaves a half-written module behind.
func WriteModule(root string, sel Selection, input Input) error {
	dir, err := moduleDir(root, sel)
	if err != nil {
		return err
	}

	reg, err := Load()
	if err != nil {
		return err
	}
	presets := make(map[string]Entry, len(input.Mounts))
	for _, name := range slices.Sorted(maps.Keys(input.Mounts)) {
		spec := input.Mounts[name]
		preset, ok := reg.Entry(spec.Type)
		if !ok {
			return fmt.Errorf("mount %q: unknown filesystem type %q (not in the generate registry)", name, spec.Type)
		}
		presets[name] = preset
	}

	// Decode the existing module.toml before any write: a corrupt file
	// must fail the run without leaving new artifacts behind.
	cfg, err := readModuleConfig(dir)
	if err != nil {
		return err
	}

	rendered := map[string]string{}
	if input.Mounts != nil {
		rendered = renderUnits(input, presets)
	}
	if input.Smb != nil {
		rendered[sharesFileName] = RenderSharesConf(*input.Smb)
	}

	if err := os.MkdirAll(dir, dirPermission); err != nil {
		return fmt.Errorf("create module dir %s: %w", dir, err)
	}
	if input.Mounts != nil {
		gcStaleUnits(dir, cfg.Dotfiles, rendered)
	}
	for _, name := range slices.Sorted(maps.Keys(rendered)) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(rendered[name]), filePermission); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Join(dir, name), err)
		}
	}
	if input.Smb != nil {
		if err := seedSmbConf(dir); err != nil {
			return err
		}
	}
	updateModuleConfig(&cfg, input, presets, rendered)
	if err := writeModuleConfig(dir, cfg); err != nil {
		return err
	}
	return nil
}

// ModuleDir resolves the target directory for the selection without
// writing. Exported for the generate wizard (internal/tui), which
// locates the target module to mark already-managed volumes.
func ModuleDir(root string, sel Selection) (string, error) {
	return moduleDir(root, sel)
}

// moduleDir resolves the target directory for the selection, validating
// the layer and the facts it needs.
func moduleDir(root string, sel Selection) (string, error) {
	if sel.ModuleID == "" {
		return "", fmt.Errorf("empty module id")
	}
	switch sel.Layer {
	case LayerBase:
		return filepath.Join(root, "modules", sel.ModuleID), nil
	case LayerHost:
		if sel.Hostname == "" {
			return "", fmt.Errorf("host layer for module %q requires a hostname", sel.ModuleID)
		}
		return filepath.Join(root, "hosts", sel.Hostname, "modules", sel.ModuleID), nil
	case LayerUser:
		if sel.Username == "" {
			return "", fmt.Errorf("user layer for module %q requires a username", sel.ModuleID)
		}
		return filepath.Join(root, "users", sel.Username, "modules", sel.ModuleID), nil
	default:
		return "", fmt.Errorf("unknown layer %q for module %q (valid: %s, %s, %s)",
			sel.Layer, sel.ModuleID, LayerBase, LayerHost, LayerUser)
	}
}

// renderUnits renders every unit file for the input mounts, keyed by file
// name. A mount always yields <stem>.mount; a StartAt additionally yields
// the oneshot <stem>.service wrapper and its <stem>.timer.
func renderUnits(input Input, presets map[string]Entry) map[string]string {
	out := make(map[string]string, 3*len(input.Mounts))
	for _, name := range slices.Sorted(maps.Keys(input.Mounts)) {
		spec := input.Mounts[name]
		stem := EscapePath(spec.Destination)
		out[stem+".mount"] = RenderMountUnit(name, spec, presets[name], input.UID, input.GID)
		if spec.StartAt != "" {
			out[stem+".service"] = RenderServiceUnit(stem + ".mount")
			out[stem+".timer"] = RenderTimerUnit(name, spec.StartAt, stem+".service")
		}
	}
	return out
}

// readModuleConfig decodes an existing module.toml in dir; a missing file
// yields the zero config.
func readModuleConfig(dir string) (profile.ModuleConfig, error) {
	var cfg profile.ModuleConfig
	path := filepath.Join(dir, moduleTomlName)
	if err := profile.DecodeModuleTOMLFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	return cfg, nil
}

// seedSmbConf writes smb.conf only when absent. Once seeded the file is
// user-owned and never clobbered.
func seedSmbConf(dir string) error {
	path := filepath.Join(dir, smbConfFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(smbConfSeed), filePermission); err != nil {
		return fmt.Errorf("seed %s: %w", path, err)
	}
	return nil
}

// updateModuleConfig applies the writer-managed sections to cfg: scope,
// mounts (wholesale replace when provided; nil Mounts means "not managed
// by this run", mirroring nil Smb), smb (wholesale replace when provided),
// packages (sorted union), and dotfile entries for every rendered file.
// All unrelated sections pass through untouched.
func updateModuleConfig(cfg *profile.ModuleConfig, input Input, presets map[string]Entry, rendered map[string]string) {
	cfg.Scope = profile.ScopeSystem
	if input.Mounts != nil {
		cfg.Mounts = input.Mounts
	}
	if input.Smb != nil {
		cfg.Smb = *input.Smb
	}
	cfg.Packages.Present = unionPackages(cfg.Packages.Present, input, presets)
	cfg.Packages.Absent = unionAbsent(cfg.Packages.Absent, presets, cfg.Packages.Present)
	if cfg.Dotfiles == nil {
		cfg.Dotfiles = make(map[string]profile.Dotfile, len(rendered)+2)
	}
	for _, name := range slices.Sorted(maps.Keys(rendered)) {
		target := unitTargetDir + "/" + name
		if name == sharesFileName {
			target = sharesTarget
		}
		cfg.Dotfiles[target] = profile.Dotfile{Source: name, Mode: dotfileMode}
	}
	if input.Smb != nil {
		cfg.Dotfiles[smbConfTarget] = profile.Dotfile{Source: smbConfFileName, Mode: dotfileMode}
	}
}

// writeModuleConfig encodes cfg as module.toml in dir. BurntSushi encodes
// maps sorted by key, so the output is deterministic for a given cfg.
func writeModuleConfig(dir string, cfg profile.ModuleConfig) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encode module config: %w", err)
	}
	path := filepath.Join(dir, moduleTomlName)
	if err := os.WriteFile(path, buf.Bytes(), filePermission); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
