package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	openusageclient "github.com/deicod/gopenusage/pkg/openusage/client"
	"github.com/spf13/cobra"
)

var (
	queryBaseURL string
	queryPlugin  string
	querySocket  string
	queryTimeout time.Duration
)

var queryCmd = &cobra.Command{
	Use:   "query [plugin-id]",
	Short: "Query the OpenUsage JSON API",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginID := strings.TrimSpace(queryPlugin)
		if len(args) == 1 {
			pluginID = strings.TrimSpace(args[0])
		}

		socketPath := resolveQuerySocketPath(cmd)

		client, err := openusageclient.New(openusageclient.Options{
			BaseURL:    queryBaseURL,
			SocketPath: socketPath,
			Timeout:    queryTimeout,
		})
		if err != nil {
			return err
		}

		var payload any
		if pluginID == "" {
			payload, err = client.QueryAll(cmd.Context())
		} else {
			payload, err = client.QueryOne(cmd.Context(), pluginID)
		}
		if err != nil {
			return err
		}

		pretty, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}

		cmd.Println(string(pretty))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)

	queryCmd.Flags().StringVar(&queryBaseURL, "url", "http://127.0.0.1:8080", "base URL of the OpenUsage API service")
	queryCmd.Flags().StringVar(&queryPlugin, "plugin", "", "plugin id to query")
	queryCmd.Flags().StringVar(&querySocket, "socket", "", "unix socket path (auto-detected when --url is not set)")
	queryCmd.Flags().DurationVar(&queryTimeout, "timeout", 15*time.Second, "request timeout")
}

func resolveQuerySocketPath(cmd *cobra.Command) string {
	socketPath := strings.TrimSpace(querySocket)
	if socketPath != "" {
		return socketPath
	}
	if cmd.Flags().Changed("url") {
		return ""
	}

	candidate := defaultSocketPath()
	info, err := os.Stat(candidate)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSocket == 0 {
		return ""
	}
	return candidate
}
