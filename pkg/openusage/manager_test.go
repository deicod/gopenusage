package openusage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

type stubPlugin struct {
	id string
	fn func(context.Context, *pluginruntime.Env) (QueryResult, error)
}

func (p stubPlugin) ID() string {
	return p.id
}

func (p stubPlugin) Query(ctx context.Context, env *pluginruntime.Env) (QueryResult, error) {
	if p.fn == nil {
		return QueryResult{}, nil
	}
	return p.fn(ctx, env)
}

func TestManagerPluginIDsOrder(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()
	writePluginManifest(t, pluginsDir, "b", "B")
	writePluginManifest(t, pluginsDir, "a", "A")

	manager, err := NewManager(Options{
		PluginsDir: pluginsDir,
		DataDir:    t.TempDir(),
	}, []Plugin{
		stubPlugin{id: "c"},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	want := []string{"a", "b", "c"}
	if got := manager.PluginIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected plugin IDs: got %v want %v", got, want)
	}
}

func TestManagerQueryOneUsesManifestAndPluginResult(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()
	dataDir := t.TempDir()
	writePluginManifest(t, pluginsDir, "copilot", "Copilot")

	var seenEnv *pluginruntime.Env
	manager, err := NewManager(Options{
		PluginsDir: pluginsDir,
		DataDir:    dataDir,
	}, []Plugin{
		stubPlugin{
			id: "copilot",
			fn: func(_ context.Context, env *pluginruntime.Env) (QueryResult, error) {
				seenEnv = env
				return QueryResult{
					Plan: "Pro",
					Lines: []MetricLine{
						NewTextLine("Status", "Active", TextLineOptions{}),
					},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	out, err := manager.QueryOne(context.Background(), "copilot")
	if err != nil {
		t.Fatalf("QueryOne error: %v", err)
	}

	if out.ProviderID != "copilot" {
		t.Fatalf("unexpected provider ID: %s", out.ProviderID)
	}
	if out.DisplayName != "Copilot" {
		t.Fatalf("unexpected display name: %s", out.DisplayName)
	}
	if out.Plan != "Pro" {
		t.Fatalf("unexpected plan: %s", out.Plan)
	}
	if out.Error != "" {
		t.Fatalf("unexpected error field: %s", out.Error)
	}
	if !strings.HasPrefix(out.IconURL, "data:image/svg+xml;base64,") {
		t.Fatalf("expected icon data URL, got %s", out.IconURL)
	}
	if len(out.Lines) != 1 || out.Lines[0].Label != "Status" {
		t.Fatalf("unexpected lines: %+v", out.Lines)
	}

	if seenEnv == nil {
		t.Fatalf("plugin env was not passed to plugin")
	}
	if seenEnv.PluginID != "copilot" {
		t.Fatalf("unexpected env plugin ID: %s", seenEnv.PluginID)
	}
	if !strings.HasPrefix(seenEnv.PluginDataDir, filepath.Join(dataDir, "plugins_data")) {
		t.Fatalf("unexpected plugin data dir: %s", seenEnv.PluginDataDir)
	}
	if _, statErr := os.Stat(seenEnv.PluginDataDir); statErr != nil {
		t.Fatalf("expected plugin data dir to exist: %v", statErr)
	}
}

func TestManagerQueryOneWithoutImplementationReturnsStructuredError(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()
	writePluginManifest(t, pluginsDir, "known", "Known")

	manager, err := NewManager(Options{
		PluginsDir: pluginsDir,
		DataDir:    t.TempDir(),
	}, nil)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	out, err := manager.QueryOne(context.Background(), "known")
	if err != nil {
		t.Fatalf("QueryOne error: %v", err)
	}

	if out.Error != "Plugin implementation unavailable" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if len(out.Lines) != 1 || out.Lines[0].Label != "Error" {
		t.Fatalf("unexpected lines for unavailable plugin: %+v", out.Lines)
	}
}

func TestManagerQueryOnePluginErrorUsesResultPayload(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()
	writePluginManifest(t, pluginsDir, "failing", "Failing")

	manager, err := NewManager(Options{
		PluginsDir: pluginsDir,
		DataDir:    t.TempDir(),
	}, []Plugin{
		stubPlugin{
			id: "failing",
			fn: func(_ context.Context, _ *pluginruntime.Env) (QueryResult, error) {
				return QueryResult{
					Plan: "Team",
					Lines: []MetricLine{
						NewBadgeLine("Warning", "Partial data", TextLineOptions{Color: "#ef4444"}),
					},
				}, errors.New("boom")
			},
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	out, err := manager.QueryOne(context.Background(), "failing")
	if err != nil {
		t.Fatalf("QueryOne error: %v", err)
	}

	if out.Error != "boom" {
		t.Fatalf("unexpected error message: %s", out.Error)
	}
	if out.Plan != "Team" {
		t.Fatalf("unexpected plan: %s", out.Plan)
	}
	if len(out.Lines) != 1 || out.Lines[0].Label != "Warning" {
		t.Fatalf("unexpected result lines: %+v", out.Lines)
	}
}

func TestManagerQueryAllWithSelectedIDs(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()
	writePluginManifest(t, pluginsDir, "a", "A")
	writePluginManifest(t, pluginsDir, "b", "B")

	manager, err := NewManager(Options{
		PluginsDir: pluginsDir,
		DataDir:    t.TempDir(),
	}, []Plugin{
		stubPlugin{
			id: "a",
			fn: func(_ context.Context, _ *pluginruntime.Env) (QueryResult, error) {
				return QueryResult{Plan: "A", Lines: []MetricLine{NewTextLine("A", "ok", TextLineOptions{})}}, nil
			},
		},
		stubPlugin{
			id: "b",
			fn: func(_ context.Context, _ *pluginruntime.Env) (QueryResult, error) {
				return QueryResult{Plan: "B", Lines: []MetricLine{NewTextLine("B", "ok", TextLineOptions{})}}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	out, err := manager.QueryAll(context.Background(), []string{"b"})
	if err != nil {
		t.Fatalf("QueryAll error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("unexpected output length: %d", len(out))
	}
	if out[0].ProviderID != "b" {
		t.Fatalf("unexpected provider: %s", out[0].ProviderID)
	}
}
