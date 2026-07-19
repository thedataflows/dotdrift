package packages_test

import (
	"context"
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
	err := b.Present(context.Background(), []string{"x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no supported package backend")
	require.Contains(t, err.Error(), "unknown")

	err = b.Absent(context.Background(), []string{"x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no supported package backend")

	ok, err := b.IsInstalled(context.Background(), "x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no supported package backend")
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
	err := b.Present(context.Background(), []string{"x"})
	require.Error(t, err, "failed auto-resolution must fail loudly, not silently no-op")
	require.Contains(t, err.Error(), "no supported package backend")
}

var errTest = errors.New("test")
