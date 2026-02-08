package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateListenerTCP(t *testing.T) {
	t.Parallel()

	listener, listenAddr, cleanup, err := createListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("createListener error: %v", err)
	}
	defer cleanup()

	if listenAddr != "127.0.0.1:0" {
		t.Fatalf("unexpected listenAddr: %s", listenAddr)
	}
	if listener.Addr().Network() != "tcp" {
		t.Fatalf("expected tcp listener, got %s", listener.Addr().Network())
	}
}

func TestCreateUnixListenerCreatesSocket(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "run", "gopenusage.sock")

	listener, listenAddr, cleanup, err := createUnixListener(socketPath)
	if err != nil {
		t.Fatalf("createUnixListener error: %v", err)
	}
	defer cleanup()

	if listenAddr != "unix://"+socketPath {
		t.Fatalf("unexpected listenAddr: %s", listenAddr)
	}
	if listener.Addr().Network() != "unix" {
		t.Fatalf("expected unix listener, got %s", listener.Addr().Network())
	}

	info, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatalf("socket path stat error: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("expected socket file mode, got: %v", info.Mode())
	}
}

func TestCreateUnixListenerRejectsNonSocketPath(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "gopenusage.sock")
	if err := os.WriteFile(socketPath, []byte("not-a-socket"), 0o644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}

	_, _, _, err := createUnixListener(socketPath)
	if err == nil {
		t.Fatalf("expected error for non-socket path")
	}
	if !strings.Contains(err.Error(), "refusing to remove non-socket path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
