package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

type apiStubPlugin struct {
	id string
}

func (p apiStubPlugin) ID() string {
	return p.id
}

func (p apiStubPlugin) Query(_ context.Context, _ *pluginruntime.Env) (openusage.QueryResult, error) {
	return openusage.QueryResult{
		Plan: "Pro",
		Lines: []openusage.MetricLine{
			openusage.NewTextLine("Status", "Active", openusage.TextLineOptions{}),
		},
	}, nil
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var payload map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !payload["ok"] {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestUsageMethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/usage", nil)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestUsageFilterByPluginIDs(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage?plugins=beta", nil)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var payload []openusage.PluginOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected exactly one plugin output, got %d", len(payload))
	}
	if payload[0].ProviderID != "beta" {
		t.Fatalf("unexpected provider ID: %s", payload[0].ProviderID)
	}
}

func TestUsageByPluginUnknown(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/missing", nil)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestUsageByPluginKnown(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage/alpha", nil)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var payload openusage.PluginOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.ProviderID != "alpha" {
		t.Fatalf("unexpected provider ID: %s", payload.ProviderID)
	}
	if payload.Plan != "Pro" {
		t.Fatalf("unexpected plan: %s", payload.Plan)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	pluginsDir := t.TempDir()
	writeManifest(t, pluginsDir, "alpha", "Alpha")
	writeManifest(t, pluginsDir, "beta", "Beta")

	manager, err := openusage.NewManager(openusage.Options{
		PluginsDir: pluginsDir,
		DataDir:    t.TempDir(),
	}, []openusage.Plugin{
		apiStubPlugin{id: "alpha"},
		apiStubPlugin{id: "beta"},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	return NewServer(manager)
}

func writeManifest(t *testing.T, pluginsDir, id, name string) {
	t.Helper()

	pluginDir := filepath.Join(pluginsDir, id)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `{"id":"` + id + `","name":"` + name + `","icon":"icon.svg"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "icon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatalf("write plugin icon: %v", err)
	}
}
