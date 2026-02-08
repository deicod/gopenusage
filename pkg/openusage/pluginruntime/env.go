package pluginruntime

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Env struct {
	PluginID      string
	DataDir       string
	PluginDataDir string
	Logger        *log.Logger
}

func DefaultDataDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil || cfgDir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil && home != "" {
			return filepath.Join(home, ".gopenusage")
		}
		return ".gopenusage"
	}
	return filepath.Join(cfgDir, "gopenusage")
}

func NewEnv(pluginID, dataDir string) (*Env, error) {
	if pluginID == "" {
		return nil, fmt.Errorf("plugin id is required")
	}
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	pluginDataDir := filepath.Join(dataDir, "plugins_data", pluginID)
	if err := os.MkdirAll(pluginDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create plugin data dir: %w", err)
	}

	logger := log.New(os.Stderr, "[plugin:"+pluginID+"] ", log.LstdFlags)

	return &Env{
		PluginID:      pluginID,
		DataDir:       dataDir,
		PluginDataDir: pluginDataDir,
		Logger:        logger,
	}, nil
}
