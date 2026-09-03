package web

import (
	"path/filepath"
	"testing"
)

func TestDefaultPersistencePathsUseDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("M365_DATA_DIR", dir)
	t.Setenv("M365_CONVERSATION_CACHE", "")
	t.Setenv("M365_API_KEYS", "")

	if got, want := dataDir(), dir; got != want {
		t.Fatalf("dataDir() = %q, want %q", got, want)
	}
	if got, want := statsPath(), filepath.Join(dir, "stats.json"); got != want {
		t.Fatalf("statsPath() = %q, want %q", got, want)
	}
	if got, want := openConversationManager().path, filepath.Join(dir, "conversations.json"); got != want {
		t.Fatalf("conversation path = %q, want %q", got, want)
	}
	if got, want := openAPIKeys().Path, filepath.Join(dir, "api-keys.json"); got != want {
		t.Fatalf("API key path = %q, want %q", got, want)
	}
}

func TestExplicitPersistencePathsTakePrecedence(t *testing.T) {
	dir := t.TempDir()
	conversationPath := filepath.Join(dir, "custom-conversations.json")
	apiKeysPath := filepath.Join(dir, "custom-api-keys.json")
	t.Setenv("M365_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("M365_CONVERSATION_CACHE", conversationPath)
	t.Setenv("M365_API_KEYS", apiKeysPath)

	if got := openConversationManager().path; got != conversationPath {
		t.Fatalf("conversation path = %q, want %q", got, conversationPath)
	}
	if got := openAPIKeys().Path; got != apiKeysPath {
		t.Fatalf("API key path = %q, want %q", got, apiKeysPath)
	}
}
