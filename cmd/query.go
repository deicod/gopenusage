package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	queryBaseURL string
	queryPlugin  string
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

		targetURL, err := buildQueryURL(queryBaseURL, pluginID)
		if err != nil {
			return err
		}

		client := &http.Client{Timeout: queryTimeout}
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
