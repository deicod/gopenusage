package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

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
		baseURL := queryBaseURL
		if socketPath != "" && !cmd.Flags().Changed("url") {
			baseURL = "http://unix"
		}

		targetURL, err := buildQueryURL(baseURL, pluginID)
		if err != nil {
			return err
		}

		client := &http.Client{Timeout: queryTimeout}
		if socketPath != "" {
			dialer := &net.Dialer{Timeout: queryTimeout}
			client.Transport = &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socketPath)
				},
			}
		}

		req, err := http.NewRequest(http.MethodGet, targetURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("invalid JSON response: %w", err)
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

func buildQueryURL(baseURL, pluginID string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "http://127.0.0.1:8080"
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid --url value: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		u.Host = u.Path
		u.Path = ""
	}

	if strings.TrimSpace(pluginID) == "" {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1/usage"
	} else {
		u.Path = strings.TrimRight(u.Path, "/") + "/v1/usage/" + strings.TrimSpace(pluginID)
	}
	return u.String(), nil
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
