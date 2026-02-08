package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deicod/gopenusage/pkg/openusage"
)

func TestQueryOne(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage/copilot" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openusage.PluginOutput{
			ProviderID:  "copilot",
			DisplayName: "Copilot",
			Plan:        "Pro",
			Lines:       []openusage.MetricLine{openusage.NewTextLine("Status", "Active", openusage.TextLineOptions{})},
		})
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	got, err := c.QueryOne(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("QueryOne error: %v", err)
	}

	if got.ProviderID != "copilot" {
		t.Fatalf("unexpected provider id: %s", got.ProviderID)
	}
	if got.Plan != "Pro" {
		t.Fatalf("unexpected plan: %s", got.Plan)
	}
	if len(got.Lines) != 1 {
		t.Fatalf("unexpected line count: %d", len(got.Lines))
	}
}

func TestQueryAll(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]openusage.PluginOutput{
			{ProviderID: "copilot", DisplayName: "Copilot"},
			{ProviderID: "codex", DisplayName: "Codex"},
		})
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	got, err := c.QueryAll(context.Background())
	if err != nil {
		t.Fatalf("QueryAll error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("unexpected result length: %d", len(got))
	}
}

func TestQueryOneReturnsAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage/missing" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown plugin"}`))
	}))
	defer srv.Close()

	c, err := New(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	_, err = c.QueryOne(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if apiErr.Message != "unknown plugin" {
		t.Fatalf("unexpected message: %s", apiErr.Message)
	}
}
