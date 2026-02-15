package cli

import (
	"testing"
)

func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic", "anthropic"},
		{"Anthropic", "anthropic"},
		{"openai", "openai"},
		{"openrouter", "openrouter"},
		{"ollama", "ollama"},
		{"1", "anthropic"},
		{"2", "openai"},
		{"3", "openrouter"},
		{"4", "ollama"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeProvider(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeProvider(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	providers := []string{"anthropic", "openai", "openrouter", "ollama"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			cfg := buildConfig(provider, "test-key", "test-agent", false, false, true)
			if cfg == nil {
				t.Fatal("buildConfig returned nil")
			}
			if len(cfg.Agents) == 0 || cfg.Agents[0].Name != "test-agent" {
				t.Errorf("agent name = %q, want %q", cfg.Agents[0].Name, "test-agent")
			}
		})
	}
}

func TestBuildConfigWithTelegram(t *testing.T) {
	cfg := buildConfig("anthropic", "sk-test", "my-agent", true, false, false)
	if cfg == nil {
		t.Fatal("buildConfig returned nil")
	}
	if len(cfg.Agents) == 0 || cfg.Agents[0].Name != "my-agent" {
		t.Errorf("unexpected agent name: %s", cfg.Agents[0].Name)
	}
}

func TestBuildConfigWithMQTT(t *testing.T) {
	cfg := buildConfig("anthropic", "sk-test", "my-agent", false, true, false)
	if cfg == nil {
		t.Fatal("buildConfig returned nil")
	}
	// When MQTT is enabled, port should be non-zero (from DefaultConfig)
	if cfg.MQTT.Port == 0 {
		t.Error("MQTT port should be non-zero when enabled")
	}
}

func TestBuildConfigWithoutMQTT(t *testing.T) {
	cfg := buildConfig("anthropic", "sk-test", "my-agent", false, false, false)
	if cfg == nil {
		t.Fatal("buildConfig returned nil")
	}
	if cfg.MQTT.Port != 0 {
		t.Errorf("MQTT port = %d, want 0 when disabled", cfg.MQTT.Port)
	}
}
