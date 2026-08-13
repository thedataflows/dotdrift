package paru

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// The paru mise package plugin is authored as real files under miseplugin/ —
// the exact layout it has in mise's plugin registry (metadata.lua,
// mise.plugin.toml, hooks/*) — and embedded at build time. This keeps the
// Lua/TOML out of Go string constants: they are edited as the files they are,
// and the file set (not a hand-maintained list) drives both WritePlugin and
// the up-to-date hash.
//
//go:embed miseplugin
var pluginFS embed.FS

// pluginRoot is the embedded plugin subtree with the "miseplugin/" prefix
// stripped, so its paths are "metadata.lua", "hooks/package_installed.lua", …
var pluginRoot = func() fs.FS {
	sub, err := fs.Sub(pluginFS, "miseplugin")
	if err != nil {
		panic(err) // unreachable: miseplugin/ is embedded above
	}
	return sub
}()

// versionRe matches the version key in metadata.lua's PLUGIN table.
var versionRe = regexp.MustCompile(`version\s*=\s*"([^"]+)"`)

// PluginVersion is the version declared in the embedded metadata.lua, parsed
// at package init so log output references the real version without a
// hand-maintained constant.
var PluginVersion = func() string {
	data, err := fs.ReadFile(pluginRoot, "metadata.lua")
	if err != nil {
		return "unknown"
	}
	m := versionRe.FindSubmatch(data)
	if len(m) < 2 {
		return "unknown"
	}
	return string(m[1])
}()

// WritePlugin writes the embedded paru plugin into dir, preserving its layout.
// Creates directories as needed; overwrites existing files (content is
// deterministic — same embed, same bytes).
func WritePlugin(dir string) error {
	return fs.WalkDir(pluginRoot, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, path)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		content, err := fs.ReadFile(pluginRoot, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		return nil
	})
}

// EnsureInstalled copies the embedded paru plugin into mise's plugin registry
// (misePluginsDir/name) as real files — the same on-disk shape as any other
// mise plugin, no symlink — but performs no writes on the common path. It
// hashes the installed plugin and compares to the embedded plugin's hash,
// recopying (remove + rewrite) only on mismatch, a missing plugin, or when the
// target is a symlink (a stale shape left by an older dotdrift). Call on every
// Arch apply; returns true when it wrote.
func EnsureInstalled(misePluginsDir, name string) (bool, error) {
	target := filepath.Join(misePluginsDir, name)
	if pluginUpToDate(target) {
		return false, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return false, fmt.Errorf("reset paru plugin: %w", err)
	}
	if err := os.MkdirAll(misePluginsDir, 0o755); err != nil {
		return false, fmt.Errorf("create mise plugins dir: %w", err)
	}
	if err := WritePlugin(target); err != nil {
		return false, err
	}
	return true, nil
}

// pluginUpToDate reports whether target is a real directory whose files hash
// to the embedded plugin's hash. A missing target, a symlink, or any read/hash
// error means not up to date (so EnsureInstalled recopies).
func pluginUpToDate(target string) bool {
	fi, err := os.Lstat(target)
	if err != nil || !fi.IsDir() {
		return false
	}
	got, err := installedHash(target)
	return err == nil && got == embeddedHash()
}

type pluginFile struct {
	path    string
	content []byte
}

// embeddedFiles lists the embedded plugin's files (path + bytes) in lexical
// order — the single source of truth for what the plugin contains, driving
// both the write and the hash.
func embeddedFiles() ([]pluginFile, error) {
	var files []pluginFile
	err := fs.WalkDir(pluginRoot, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(pluginRoot, path)
		if err != nil {
			return err
		}
		files = append(files, pluginFile{path: path, content: b})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, err
}

// embeddedHash is the SHA-256 of the embedded plugin's path:content pairs.
func embeddedHash() string {
	files, err := embeddedFiles()
	if err != nil {
		panic(err) // unreachable: the FS is embedded
	}
	return hashFiles(files)
}

// installedHash hashes the plugin files present under dir over the same
// path:content pairs as embeddedHash. A missing/unreadable file yields an
// error so the caller treats the plugin as out of date.
func installedHash(dir string) (string, error) {
	want, err := embeddedFiles()
	if err != nil {
		return "", err
	}
	files := make([]pluginFile, 0, len(want))
	for _, f := range want {
		b, err := os.ReadFile(filepath.Join(dir, f.path))
		if err != nil {
			return "", err
		}
		files = append(files, pluginFile{path: f.path, content: b})
	}
	return hashFiles(files), nil
}

func hashFiles(files []pluginFile) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.path))
		h.Write([]byte{0})
		h.Write(f.content)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
