package pluginruntime

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type HTTPRequest struct {
	Method               string
	URL                  string
	Headers              map[string]string
	BodyText             string
	Timeout              time.Duration
	DangerouslyIgnoreTLS bool
}

type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"bodyText"`
}

func DoHTTPRequest(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if req.DangerouslyIgnoreTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, bytes.NewBufferString(req.BodyText))
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("build request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("pluginruntime: close response body: %v", closeErr)
		}
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("read body: %w", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for k, values := range resp.Header {
		if len(values) == 0 {
			continue
		}
		headers[strings.ToLower(k)] = values[0]
	}

	return HTTPResponse{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(bodyBytes),
	}, nil
}
