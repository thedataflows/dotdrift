package mise

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultRunContext_errorIncludesOutput(t *testing.T) {
	_, err := defaultRunContext(context.Background(), "sh", "-c", "echo boom-output; exit 1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "exit status 1")
	require.Contains(t, err.Error(), "boom-output")
}

func TestDefaultRunContext_ctxCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := defaultRunContext(ctx, "sh", "-c", "sleep 30")
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second, "cancelled ctx must kill the process promptly")
}

func TestInstallFailure_hintsNetworkAndPreseed(t *testing.T) {
	err := installFailure("/home/u/.local/bin/mise", errors.New("exit status 7"), []byte("curl: (7) failed to connect"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "curl: (7) failed to connect")
	require.Contains(t, err.Error(), "network connectivity")
	require.Contains(t, err.Error(), "pre-seed")
	require.Contains(t, err.Error(), ".local/bin/mise")
}
