package mise_test

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
)

func TestPrefixedPackages_bareGetsBackendPrefix(t *testing.T) {
	got := mise.PrefixedPackages([]string{"neovim", "ripgrep"}, "paru")
	require.Equal(t, []string{"paru:neovim", "paru:ripgrep"}, got)
}

func TestPrefixedPackages_aptBackend(t *testing.T) {
	got := mise.PrefixedPackages([]string{"curl"}, "apt")
	require.Equal(t, []string{"apt:curl"}, got)
}

func TestPrefixedPackages_explicitPrefixPassesThrough(t *testing.T) {
	got := mise.PrefixedPackages([]string{"pacman:foo", "neovim"}, "paru")
	require.Equal(t, []string{"pacman:foo", "paru:neovim"}, got)
}

func TestPrefixedPackages_empty(t *testing.T) {
	require.Empty(t, mise.PrefixedPackages(nil, "paru"))
}

func TestPrefixedPackages_dnfBackend(t *testing.T) {
	got := mise.PrefixedPackages([]string{"openssl-devel"}, "dnf")
	require.Equal(t, []string{"dnf:openssl-devel"}, got)
}
// TestPrefixedPackages_aurMarkerMapsToParu covers dotdrift's AUR notation:
// `aur/<pkg>` has no colon, so it is not an explicit manager prefix — it maps
// to the paru plugin (the only AUR-capable manager) with the marker stripped,
// so pacman -Q / paru -S receive the real package name. This is independent of
// the detected backend: an AUR package needs paru regardless.
func TestPrefixedPackages_aurMarkerMapsToParu(t *testing.T) {
	got := mise.PrefixedPackages([]string{"aur/fresh-editor-bin"}, "paru")
	require.Equal(t, []string{"paru:fresh-editor-bin"}, got)
}

func TestPrefixedPackages_aurMarkerStripsEvenOnNonArchBackend(t *testing.T) {
	// aur/ is an explicit AUR signal: it forces paru even if the detected
	// backend were something else (defensive — aur/ only appears in Arch modules).
	got := mise.PrefixedPackages([]string{"aur/lmstudio-bin"}, "apt")
	require.Equal(t, []string{"paru:lmstudio-bin"}, got)
}

func TestPrefixedPackages_mixedAurBareExplicit(t *testing.T) {
	got := mise.PrefixedPackages([]string{"aur/fresh-editor-bin", "ripgrep", "pacman:base-devel"}, "paru")
	require.Equal(t, []string{"paru:fresh-editor-bin", "paru:ripgrep", "pacman:base-devel"}, got)
}

func TestGenerateBootstrapPackages_emitsSection(t *testing.T) {
	got := mise.GenerateBootstrapPackages([]string{"neovim", "ripgrep"}, "paru")
	require.Contains(t, got, "[bootstrap.packages]")
	require.Contains(t, got, `"paru:neovim" = "latest"`)
	require.Contains(t, got, `"paru:ripgrep" = "latest"`)
}

func TestGenerateBootstrapPackages_deterministicOrder(t *testing.T) {
	a := mise.GenerateBootstrapPackages([]string{"zsh", "bash", "curl"}, "apt")
	b := mise.GenerateBootstrapPackages([]string{"curl", "zsh", "bash"}, "apt")
	require.Equal(t, a, b)
	// sorted: apt:bash, apt:curl, apt:zsh
	lines := strings.Split(strings.TrimSpace(a), "\n")
	require.Equal(t, "[bootstrap.packages]", lines[0])
	require.Equal(t, `"apt:bash" = "latest"`, lines[1])
}

func TestGenerateBootstrapPackages_emptyReturnsEmpty(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapPackages(nil, "paru"))
}

func TestGenerateBootstrapPackages_prefixedPassesThrough(t *testing.T) {
	got := mise.GenerateBootstrapPackages([]string{"pacman:foo", "neovim"}, "paru")
	require.Contains(t, got, `"pacman:foo" = "latest"`)
	require.Contains(t, got, `"paru:neovim" = "latest"`)
}

// TestGenerateBootstrapConfig_validTOML verifies the generated config parses
// as valid TOML — escaping/quoting issues would make mise reject it.
func TestGenerateBootstrapConfig_validTOML(t *testing.T) {
	content := mise.GenerateBootstrapConfig(
		[]string{"neovim", "ripgrep", "pacman:base-devel"},
		"paru",
		"/home/user/.local/share/dotdrift/mise-plugins/paru",
	)
	require.NotEmpty(t, content)

	var parsed map[string]any
	err := toml.Unmarshal([]byte(content), &parsed)
	require.NoError(t, err, "generated config must be valid TOML:\n%s", content)

	pkgs, ok := parsed["bootstrap"].(map[string]any)
	require.True(t, ok, "expected [bootstrap] section")
	packages, ok := pkgs["packages"].(map[string]any)
	require.True(t, ok, "expected [bootstrap.packages]")
	require.Contains(t, packages, "paru:neovim")
	require.Contains(t, packages, "pacman:base-devel")
	plugins, ok := pkgs["plugins"].(map[string]any)
	require.True(t, ok, "expected [bootstrap.plugins]")
	require.Contains(t, plugins, "paru")
}
