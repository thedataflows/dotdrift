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
