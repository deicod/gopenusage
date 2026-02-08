package openusage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type ManifestLine struct {
	Type         string `json:"type"`
	Label        string `json:"label"`
	Scope        string `json:"scope"`
	PrimaryOrder *int   `json:"primaryOrder,omitempty"`
}

type PluginManifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Entry         string         `json:"entry"`
	Icon          string         `json:"icon"`
	BrandColor    string         `json:"brandColor,omitempty"`
	Lines         []ManifestLine `json:"lines"`
}

type LoadedManifest struct {
	Manifest    PluginManifest
	PluginDir   string
	IconDataURL string
}

func LoadManifests(pluginsDir string) (map[string]LoadedManifest, []string, error) {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read plugins dir: %w", err)
	}

	manifests := make(map[string]LoadedManifest)
	order := make([]string, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "plugin.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest PluginManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			continue
		}
		if manifest.ID == "" {
			continue
		}

		iconDataURL := ""
		if manifest.Icon != "" {
			iconPath := filepath.Join(pluginDir, manifest.Icon)
			iconBytes, err := os.ReadFile(iconPath)
			if err == nil {
				iconDataURL = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(iconBytes)
			}
		}

		manifests[manifest.ID] = LoadedManifest{
			Manifest:    manifest,
			PluginDir:   pluginDir,
			IconDataURL: iconDataURL,
		}
		order = append(order, manifest.ID)
	}

	sort.Strings(order)
	return manifests, order, nil
}
