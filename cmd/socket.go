package cmd

import (
	"os"
	"path/filepath"
	"strconv"
)

func defaultSocketPath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "gopenusage", "gopenusage.sock")
	}

	uid := os.Getuid()
	if uid > 0 {
		runUserDir := filepath.Join("/run/user", strconv.Itoa(uid))
		if info, err := os.Stat(runUserDir); err == nil && info.IsDir() {
			return filepath.Join(runUserDir, "gopenusage", "gopenusage.sock")
		}
	}

	return filepath.Join(os.TempDir(), "gopenusage", "gopenusage.sock")
}

func defaultServeAddr() string {
	return "unix://" + defaultSocketPath()
}
