package openusage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadManifestsLoadsAndSorts(t *testing.T) {
	t.Parallel()

	pluginsDir := t.TempDir()

	writePluginManifest(t, pluginsDir, "zeta", "Zeta")
	writePluginManifest(t, pluginsDir, "alpha", "Alpha")

	invalidDir := filepath.Join(pluginsDir, "invalid")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatalf("mkdir invalid plugin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "plugin.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write invalid plugin manifest: %v", err)
	}

	manifests, order, err := LoadManifests(pluginsDir)
	if err != nil {
		t.Fatalf("LoadManifests error: %v", err)
	}

	wantOrder := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("unexpected manifest order: got %v want %v", order, wantOrder)
	}

	if len(manifests) != 2 {
		t.Fatalf("unexpected manifest count: %d", len(manifests))
	}

	alpha := manifests["alpha"]
	if alpha.Manifest.Name != "Alpha" {
		t.Fatalf("unexpected alpha name: %s", alpha.Manifest.Name)
	}
	if !strings.HasPrefix(alpha.IconDataURL, "data:image/svg+xml;base64,") {
		t.Fatalf("expected icon data URL prefix, got: %s", alpha.IconDataURL)
	}
}

func TestLoadManifestsReturnsErrorForMissingDirectory(t *testing.T) {
	t.Parallel()

	_, _, err := LoadManifests(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("expected error for missing directory")
	}
}

func writePluginManifest(t *testing.T, pluginsDir, id, name string) {
	t.Helper()

	pluginDir := filepath.Join(pluginsDir, id)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir %s: %v", id, err)
	}

	manifest := `{"id":"` + id + `","name":"` + name + `","icon":"icon.svg"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest %s: %v", id, err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "icon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatalf("write icon %s: %v", id, err)
	}
}
