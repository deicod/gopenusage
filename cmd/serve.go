package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/deicod/gopenusage/internal/api"
	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/builtin"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
	"github.com/spf13/cobra"
)

var (
	serveAddr       string
	servePluginsDir string
	serveDataDir    string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the OpenUsage JSON API service",
	RunE: func(cmd *cobra.Command, _ []string) error {
		manager, err := openusage.NewManager(openusage.Options{
			PluginsDir: servePluginsDir,
			DataDir:    serveDataDir,
		}, builtin.Plugins())
		if err != nil {
			return err
		}

		server := api.NewServer(manager)
		httpServer := &http.Server{
			Addr:    serveAddr,
			Handler: server.Handler(),
		}

		listener, listenAddr, cleanup, err := createListener(serveAddr)
		if err != nil {
			return err
		}
		defer cleanup()

		cmd.Printf("listening on %s\n", listenAddr)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&serveAddr, "addr", defaultServeAddr(), "listen address (e.g. :8080 or unix:///path/to.sock)")
	serveCmd.Flags().StringVar(&servePluginsDir, "plugins-dir", "openusage/plugins", "path to plugin manifests")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", pluginruntime.DefaultDataDir(), "state directory for plugin data")
}

func createListener(rawAddr string) (net.Listener, string, func(), error) {
	addr := strings.TrimSpace(rawAddr)
	if addr == "" {
		addr = defaultServeAddr()
	}

	if strings.HasPrefix(addr, "unix://") {
		socketPath := strings.TrimSpace(strings.TrimPrefix(addr, "unix://"))
		return createUnixListener(socketPath)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", nil, fmt.Errorf("listen on %q: %w", addr, err)
	}
	cleanup := func() { _ = listener.Close() }
	return listener, addr, cleanup, nil
}

func createUnixListener(socketPath string) (net.Listener, string, func(), error) {
	if socketPath == "" {
		return nil, "", nil, fmt.Errorf("unix socket path is required")
	}

	parent := filepath.Dir(socketPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, "", nil, fmt.Errorf("create socket directory %q: %w", parent, err)
	}

	info, err := os.Lstat(socketPath)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, "", nil, fmt.Errorf("refusing to remove non-socket path %q", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, "", nil, fmt.Errorf("remove stale socket %q: %w", socketPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, "", nil, fmt.Errorf("inspect socket path %q: %w", socketPath, err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("listen on unix socket %q: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, "", nil, fmt.Errorf("set socket permissions: %w", err)
	}

	cleanup := func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}
	return listener, "unix://" + socketPath, cleanup, nil
}
