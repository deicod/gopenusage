package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveQuerySocketPathExplicitValue(t *testing.T) {
	prev := querySocket
	querySocket = "/tmp/custom.sock"
	t.Cleanup(func() { querySocket = prev })

	cmd := newTestQueryCommand(t)
	got := resolveQuerySocketPath(cmd)
	if got != "/tmp/custom.sock" {
		t.Fatalf("unexpected socket path: %s", got)
	}
}

func TestResolveQuerySocketPathIgnoresAutoWhenURLSet(t *testing.T) {
	prev := querySocket
	querySocket = ""
	t.Cleanup(func() { querySocket = prev })

	cmd := newTestQueryCommand(t)
	if err := cmd.Flags().Set("url", "http://127.0.0.1:18080"); err != nil {
		t.Fatalf("set url flag: %v", err)
	}

	if got := resolveQuerySocketPath(cmd); got != "" {
		t.Fatalf("expected empty socket path, got %s", got)
	}
}

func TestResolveQuerySocketPathAutoDetectsSocket(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	prev := querySocket
	querySocket = ""
	t.Cleanup(func() { querySocket = prev })

	socketPath := defaultSocketPath()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("mkdir socket parent: %v", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("create socket: %v", err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	})

	cmd := newTestQueryCommand(t)
	got := resolveQuerySocketPath(cmd)
	if got != socketPath {
		t.Fatalf("unexpected auto-detected socket path: got %s want %s", got, socketPath)
	}
}

func newTestQueryCommand(t *testing.T) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("url", "", "base URL")
	return cmd
}
