package pluginruntime

import (
	"os"
	"path/filepath"
	"strings"
)

func ExpandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func FileExists(path string) bool {
	_, err := os.Stat(ExpandPath(path))
	return err == nil
}

func ReadText(path string) (string, error) {
	data, err := os.ReadFile(ExpandPath(path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteText(path, content string) error {
	expanded := ExpandPath(path)
	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return err
	}
	return os.WriteFile(expanded, []byte(content), 0o600)
}
