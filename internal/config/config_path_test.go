package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathUsesDataDirWhenConfigUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("M365_CONFIG", "")
	t.Setenv("M365_DATA_DIR", dir)

	if got, want := Path(), filepath.Join(dir, "accounts.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestPathExplicitConfigTakesPrecedence(t *testing.T) {
	t.Setenv("M365_DATA_DIR", t.TempDir())
	t.Setenv("M365_CONFIG", filepath.Join(t.TempDir(), "accounts.json"))

	if got, want := Path(), filepath.Join(filepath.Dir(os.Getenv("M365_CONFIG")), "accounts.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
