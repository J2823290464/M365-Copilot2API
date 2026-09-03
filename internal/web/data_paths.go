package web

import (
	"os"
	"path/filepath"
	"strings"
)

func dataDir() string {
	if dir := strings.TrimSpace(os.Getenv("M365_DATA_DIR")); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return "."
	}
	return filepath.Join(home, ".config", "m365-copilot2api")
}

func dataPath(name string) string {
	return filepath.Join(dataDir(), name)
}
