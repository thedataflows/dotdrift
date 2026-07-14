package packages_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

func TestFor_paru(t *testing.T) {
	require.IsType(t, &packages.Paru{}, packages.For("paru"))
}

func TestFor_apt(t *testing.T) {
	require.IsType(t, &packages.Apt{}, packages.For("apt"))
}

func TestFor_dnf(t *testing.T) {
	require.IsType(t, &packages.Dnf{}, packages.For("dnf"))
}

func TestFor_unknown(t *testing.T) {
	b := packages.For("unknown")
	require.NoError(t, b.Present(nil))
	require.NoError(t, b.Absent(nil))
	ok, err := b.IsInstalled("")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestFor_auto(t *testing.T) {
	old := packages.AutoBackend
	packages.AutoBackend = func() (string, error) { return "apt", nil }
	defer func() { packages.AutoBackend = old }()
	require.IsType(t, &packages.Apt{}, packages.For("auto"))
}

func TestFor_auto_fallbackOnError(t *testing.T) {
	old := packages.AutoBackend
	packages.AutoBackend = func() (string, error) { return "", errTest }
	defer func() { packages.AutoBackend = old }()
	b := packages.For("auto")
	require.NoError(t, b.Present(nil))
}

var errTest = errors.New("test")
