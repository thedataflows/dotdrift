package mise_test

import (
	"strings"
	"testing"

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
