package paru

import (
	"fmt"
	"os"
	"path/filepath"
)

// misePluginToml is the [package-manager] config that tells mise this is a
// package plugin requiring the `paru` host binary on Linux.
const misePluginToml = `[package-manager]
requires = ["paru"]
os = ["linux"]
`
// metadataLua assigns the global PLUGIN table that mise's vfox loader expects
// (load_metadata runs `require "metadata"; return PLUGIN`). A metadata.lua that
// only `return`s a table leaves PLUGIN nil, so the package hooks never attach
// and mise reports the plugin "not installed" — the contradictory warnings.
// `name` is required by mise's Metadata deserializer; description/version are
// informational.
const metadataLua = `PLUGIN = {
  name = "paru",
  description = "paru (pacman + AUR) package manager",
  version = "1.0.0",
}
`

// packageInstalledLua delegates to `dotdrift paru installed` and parses the
// line-based status protocol into the vfox response table.
const packageInstalledLua = `function PLUGIN:PackageInstalled(ctx)
  local names = {}
  for _, pkg in ipairs(ctx.packages) do
    table.insert(names, pkg.name)
  end
  local handle = io.popen("dotdrift paru installed " .. table.concat(names, " "))
  local output = handle:read("*a")
  handle:close()
  local result = { packages = {} }
  for line in output:gmatch("[^\n]+") do
    local name, state, version = line:match("([^\t]+)\t([^\t]+)\t?([^\t]*)")
    if name and state then
      local entry = { name = name, state = state }
      if version and version ~= "" then entry.version = version end
      table.insert(result.packages, entry)
    end
  end
  return result
end
`

// packageInstallLua delegates to `dotdrift paru install` with dry-run/update
// flags forwarded from the vfox ctx.
const packageInstallLua = `function PLUGIN:PackageInstall(ctx)
  local names = {}
  for _, pkg in ipairs(ctx.packages) do
    table.insert(names, pkg.name)
  end
  local flags = ""
  if ctx.dry_run then flags = flags .. " --dry-run" end
  if ctx.update then flags = flags .. " --update" end
  os.execute("dotdrift paru install" .. flags .. " " .. table.concat(names, " "))
  return {}
end
`

// WritePlugin writes the mise package-plugin for the `paru` manager into dir.
// Creates the directory and hooks/ subdirectory. Overwrites existing files
// (the content is deterministic — same call, same bytes).
func WritePlugin(dir string) error {
	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create plugin hooks dir: %w", err)
	}
	files := map[string]string{
		"metadata.lua":                metadataLua,
		"mise.plugin.toml":            misePluginToml,
		"hooks/package_installed.lua": packageInstalledLua,
		"hooks/package_install.lua":   packageInstallLua,
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// EnsureInstalled makes the on-disk paru plugin an exact mirror of the embedded
// plugin for the running dotdrift version and guarantees mise's plugin
// registry symlinks it. Idempotent and self-repairing — call on every Arch
// apply. dotdrift owns the plugin, so it does not rely on mise's passive
// bootstrap linking (which prints "already installed" and skips a stale or
// mis-pointing link):
//   - the source dir is reset and rewritten, so a file dropped by a newer
//     dotdrift cannot linger from an older one (exact mirror);
//   - the mise registry symlink is created, or repaired when missing,
//     dangling, or pointing at the wrong target. A non-symlink entry (e.g. a
//     git-cloned plugin) is left in place — it is not dotdrift's to delete.
func EnsureInstalled(sourceDir, misePluginsDir string) error {
	if err := os.RemoveAll(sourceDir); err != nil {
		return fmt.Errorf("reset paru plugin source: %w", err)
	}
	if err := WritePlugin(sourceDir); err != nil {
		return err
	}
	return ensureSymlink(filepath.Join(misePluginsDir, filepath.Base(sourceDir)), sourceDir)
}

// ensureSymlink makes link a symlink to target. A correct link is left as-is;
// a missing, dangling, or mis-pointing symlink is (re)created; a non-symlink
// entry (real file/dir) is untouched (not dotdrift's to remove).
func ensureSymlink(link, target string) error {
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			return nil // real file/dir — leave it
		}
		if got, err := os.Readlink(link); err == nil && got == target {
			return nil // already correct
		}
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove stale paru plugin link: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat paru plugin link: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("create mise plugins dir: %w", err)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("link paru plugin into mise: %w", err)
	}
	return nil
}
