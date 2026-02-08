package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
)

const defaultBaseURL = "http://127.0.0.1:8080"

type Options struct {
	BaseURL    string
	SocketPath string
	Timeout    time.Duration
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("request failed (HTTP %d): %s", e.StatusCode, e.Message)
}

func New(opts Options) (*Client, error) {
	base, err := normalizeBaseURL(opts.BaseURL, opts.SocketPath != "")
	if err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	httpClient := &http.Client{Timeout: timeout}
	if strings.TrimSpace(opts.SocketPath) != "" {
		socketPath := strings.TrimSpace(opts.SocketPath)
		dialer := &net.Dialer{Timeout: timeout}
		httpClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		}
	}

	return &Client{
		baseURL:    base,
		httpClient: httpClient,
	}, nil
}

func (c *Client) QueryAll(ctx context.Context) ([]openusage.PluginOutput, error) {
	var output []openusage.PluginOutput
	if err := c.getJSON(ctx, "/v1/usage", nil, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (c *Client) QueryPlugins(ctx context.Context, pluginIDs []string) ([]openusage.PluginOutput, error) {
	filtered := make([]string, 0, len(pluginIDs))
	for _, id := range pluginIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return c.QueryAll(ctx)
	}

	query := url.Values{}
	query.Set("plugins", strings.Join(filtered, ","))

	var output []openusage.PluginOutput
	if err := c.getJSON(ctx, "/v1/usage", query, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (c *Client) QueryOne(ctx context.Context, pluginID string) (openusage.PluginOutput, error) {
	id := strings.TrimSpace(pluginID)
	if id == "" {
		return openusage.PluginOutput{}, fmt.Errorf("plugin id is required")
	}

	var output openusage.PluginOutput
	if err := c.getJSON(ctx, "/v1/usage/"+url.PathEscape(id), nil, &output); err != nil {
		return openusage.PluginOutput{}, err
	}
	return output, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, target any) error {
	targetURL := *c.baseURL
	targetURL.Path = strings.TrimRight(targetURL.Path, "/") + path
	targetURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    errorMessage(body),
		}
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	return nil
}

func normalizeBaseURL(rawBaseURL string, hasSocket bool) (*url.URL, error) {
	base := strings.TrimSpace(rawBaseURL)
	if base == "" {
		if hasSocket {
			base = "http://unix"
		} else {
			base = defaultBaseURL
		}
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		u.Host = u.Path
		u.Path = ""
	}
	return u, nil
}

func errorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return strings.TrimSpace(payload.Error)
	}
	return trimmed
}
