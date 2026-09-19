package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLimitToolCalls(t *testing.T) {
	calls := []detectedToolCall{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := limitToolCalls(calls, 1)
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("got %#v", got)
	}
	if len(limitToolCalls(calls, 2)) != 2 {
		t.Fatal("expected two calls")
	}
	if len(limitToolCalls(calls, 99)) != 3 {
		t.Fatal("must preserve calls below limit")
	}
}
func TestSettingsPersistAndValidate(t *testing.T) {
	s := &settingsStore{path: filepath.Join(t.TempDir(), "settings.json"), v: defaultRuntimeSettings()}
	v := s.v
	v.MaxToolCallsPerTurn = 1
	v.MaxToolRounds = 32
	v.ChatTimeoutSeconds = 60
	v.ImageTimeoutSeconds = 90
	if err := s.save(v); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path); err != nil {
		t.Fatal(err)
	}
	v.MaxToolCallsPerTurn = 0
	if err := s.save(v); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestModelMappingsValidate(t *testing.T) {
	v := defaultRuntimeSettings()
	v.ModelMappings = []modelMapping{{PublicModel: "gpt-5.6-sol", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6-Sol", DefaultReasoningLevel: "low"}}
	if err := validateSettings(v); err != nil {
		t.Fatal(err)
	}
	v.ModelMappings[0].UpstreamTone = "unknown"
	if err := validateSettings(v); err == nil {
		t.Fatal("accepted unknown upstream tone")
	}
	v.ModelMappings[0].UpstreamTone = "Gpt_5_6_Reasoning"
	v.ModelMappings = append(v.ModelMappings, v.ModelMappings[0])
	if err := validateSettings(v); err == nil {
		t.Fatal("accepted duplicate public model")
	}
	v.ModelMappings = []modelMapping{{PublicModel: "custom-codex-route", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "Custom Codex Route", DefaultReasoningLevel: "medium"}}
	if err := validateSettings(v); err != nil {
		t.Fatalf("rejected custom public model: %v", err)
	}
}

func TestOutboundProxySettingValidation(t *testing.T) {
	v := defaultRuntimeSettings()
	v.OutboundProxy = "socks5://proxy.example:1080"
	if err := validateSettings(v); err != nil {
		t.Fatalf("rejected SOCKS5 proxy: %v", err)
	}
	v.OutboundProxy = "https://proxy.example:8443"
	if err := validateSettings(v); err != nil {
		t.Fatalf("rejected HTTPS proxy: %v", err)
	}
	v.OutboundProxy = "ftp://proxy.example:21"
	if err := validateSettings(v); err == nil {
		t.Fatal("accepted unsupported proxy scheme")
	}
}

func TestAdaptiveToolCallLimitSerializesDependentOrMutatingCalls(t *testing.T) {
	calls := []detectedToolCall{{Name: "read_file"}, {Name: "exec_command"}}
	if got := adaptiveToolCallLimit(calls, 4); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}
func TestAdaptiveToolCallLimitAllowsIndependentReadOnlyCalls(t *testing.T) {
	calls := []detectedToolCall{{Name: "read_file"}, {Name: "search_code"}}
	if got := adaptiveToolCallLimit(calls, 4); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}
}

func TestClientToolPermissionValidation(t *testing.T) {
	v := defaultRuntimeSettings()
	for _, permission := range []string{"default", "auto_review", "full_access"} {
		v.ClientToolPermission = permission
		if err := validateSettings(v); err != nil {
			t.Fatalf("permission %s rejected: %v", permission, err)
		}
	}
	v.ClientToolPermission = "invalid"
	if err := validateSettings(v); err == nil {
		t.Fatal("invalid permission accepted")
	}
}

func TestMaxRequestPayloadBytesDefaultAndValidation(t *testing.T) {
	v := defaultRuntimeSettings()
	if v.MaxRequestPayloadBytes != defaultMaxRequestPayloadBytes {
		t.Fatalf("default payload bytes = %d, want %d", v.MaxRequestPayloadBytes, defaultMaxRequestPayloadBytes)
	}
	v.MaxRequestPayloadBytes = 512 * 1024
	if err := validateSettings(v); err != nil {
		t.Fatalf("512KiB rejected: %v", err)
	}
	v.MaxRequestPayloadBytes = 32 * 1024
	if err := validateSettings(v); err == nil {
		t.Fatal("32KiB accepted, want validation error")
	}
	v.MaxRequestPayloadBytes = 20 * 1024 * 1024
	if err := validateSettings(v); err == nil {
		t.Fatal("20MiB accepted, want validation error")
	}
}

func TestMaxRequestPayloadBytesEnvOverride(t *testing.T) {
	t.Setenv("M365_MAX_REQUEST_PAYLOAD_BYTES", "524288")
	v := defaultRuntimeSettings()
	if v.MaxRequestPayloadBytes != 512*1024 {
		t.Fatalf("env override = %d, want 524288", v.MaxRequestPayloadBytes)
	}
}
