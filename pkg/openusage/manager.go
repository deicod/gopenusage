package openusage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

type Plugin interface {
	ID() string
	Query(ctx context.Context, env *pluginruntime.Env) (QueryResult, error)
}

type Options struct {
	PluginsDir string
	DataDir    string
}

type Manager struct {
	plugins   map[string]Plugin
	manifests map[string]LoadedManifest
	order     []string
	dataDir   string
}

func NewManager(opts Options, plugins []Plugin) (*Manager, error) {
	pluginsDir := opts.PluginsDir
	pluginsDirExplicit := pluginsDir != ""
	if pluginsDir == "" {
		pluginsDir = filepath.Join("openusage", "plugins")
	}

	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = pluginruntime.DefaultDataDir()
	}

	manifestMap := make(map[string]LoadedManifest)
	manifestOrder := make([]string, 0)
	loadedManifests, loadedOrder, err := LoadManifests(pluginsDir)
	if err != nil {
		// Manifests are optional when using defaults. Plugin implementations are Go-native.
		if pluginsDirExplicit || !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load manifests from %s: %w", pluginsDir, err)
		}
	} else {
		manifestMap = loadedManifests
		manifestOrder = loadedOrder
	}

	pluginMap := make(map[string]Plugin, len(plugins))
	for _, p := range plugins {
		pluginMap[p.ID()] = p
	}

	order := make([]string, 0, len(manifestOrder)+len(pluginMap))
	seen := make(map[string]struct{}, len(manifestOrder)+len(pluginMap))
	for _, id := range manifestOrder {
		order = append(order, id)
		seen[id] = struct{}{}
	}

	extra := make([]string, 0)
	for id := range pluginMap {
		if _, ok := seen[id]; !ok {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)

	return &Manager{
		plugins:   pluginMap,
		manifests: manifestMap,
		order:     order,
		dataDir:   dataDir,
	}, nil
}

func (m *Manager) PluginIDs() []string {
	ids := make([]string, len(m.order))
	copy(ids, m.order)
	return ids
}

func (m *Manager) HasPlugin(id string) bool {
	if _, ok := m.plugins[id]; ok {
		return true
	}
	if _, ok := m.manifests[id]; ok {
		return true
	}
	return false
}

func (m *Manager) QueryAll(ctx context.Context, ids []string) ([]PluginOutput, error) {
	targetIDs := ids
	if len(targetIDs) == 0 {
		targetIDs = m.PluginIDs()
	}

	out := make([]PluginOutput, 0, len(targetIDs))
	for _, id := range targetIDs {
		result, err := m.QueryOne(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}

func (m *Manager) QueryOne(ctx context.Context, id string) (PluginOutput, error) {
	manifest, hasManifest := m.manifests[id]

	output := PluginOutput{
		ProviderID:  id,
		DisplayName: id,
		Lines:       ErrorLines("No data"),
	}

	if hasManifest {
		if manifest.Manifest.Name != "" {
			output.DisplayName = manifest.Manifest.Name
		}
		output.IconURL = manifest.IconDataURL
	}

	plugin, ok := m.plugins[id]
	if !ok {
		errMsg := "Plugin implementation unavailable"
		output.Error = errMsg
		output.Lines = ErrorLines(errMsg)
		return output, nil
	}

	env, err := pluginruntime.NewEnv(id, m.dataDir)
	if err != nil {
		return PluginOutput{}, fmt.Errorf("init env for %s: %w", id, err)
	}

	result, err := plugin.Query(ctx, env)
	if err != nil {
		output.Error = err.Error()
		output.Lines = ErrorLines(err.Error())
		if result.Plan != "" {
			output.Plan = result.Plan
		}
		if len(result.Lines) > 0 {
			output.Lines = result.Lines
		}
		return output, nil
	}

	output.Plan = result.Plan
	if len(result.Lines) == 0 {
		output.Lines = ErrorLines("No usage data")
	} else {
		output.Lines = result.Lines
	}

	return output, nil
}
