package cmd

import (
	"fmt"
	"net/http"

	"github.com/deicod/copilot-usage/internal/api"
	"github.com/deicod/copilot-usage/pkg/openusage"
	"github.com/deicod/copilot-usage/pkg/openusage/builtin"
	"github.com/deicod/copilot-usage/pkg/openusage/pluginruntime"
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

		cmd.Printf("listening on %s\n", serveAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "listen address")
	serveCmd.Flags().StringVar(&servePluginsDir, "plugins-dir", "openusage/plugins", "path to plugin manifests")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", pluginruntime.DefaultDataDir(), "state directory for plugin data")
}
