package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupCLI_NoArgs(t *testing.T) {
	cli := NewSetupCLI()
	code := cli.Run([]string{})
	if code != 1 {
		t.Errorf("expected exit code 1 with no args, got %d", code)
	}
}

func TestSetupCLI_Help(t *testing.T) {
	cli := NewSetupCLI()
	code := cli.Run([]string{"help"})
	if code != 0 {
		t.Errorf("expected exit code 0 for help, got %d", code)
	}
}

func TestSetupCLI_HelpFlag(t *testing.T) {
	cli := NewSetupCLI()
	code := cli.Run([]string{"--help"})
	if code != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", code)
	}
}

func TestSetupCLI_UnknownCommand(t *testing.T) {
	cli := NewSetupCLI()
	code := cli.Run([]string{"unknown"})
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", code)
	}
}

func TestSetupCLI_Hub_GeneratesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")
	dataDir := filepath.Join(tmpDir, "data")

	cli := NewSetupCLI()
	code := cli.Run([]string{"hub",
		"--config", configPath,
		"--data-dir", dataDir,
		"--port", "9999",
		"--mqtt-port", "2883",
	})

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	// Verify config file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("expected config file to exist")
	}

	// Verify data directory was created
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Fatal("expected data directory to exist")
	}
}

func TestSetupCLI_Hub_ExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	// Create a valid config file
	validConfig := `{"server":{"port":8420,"dataDir":"./data","logLevel":"info"},"mqtt":{"port":1883,"host":"0.0.0.0"}}`
	os.WriteFile(configPath, []byte(validConfig), 0640)

	cli := NewSetupCLI()
	code := cli.Run([]string{"hub", "--config", configPath, "--data-dir", filepath.Join(tmpDir, "data")})

	if code != 0 {
		t.Errorf("expected exit code 0 with existing config, got %d", code)
	}
}

func TestSetupCLI_Hub_InvalidExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	// Create an invalid config file
	os.WriteFile(configPath, []byte("not valid json {{{"), 0640)

	cli := NewSetupCLI()
	code := cli.Run([]string{"hub", "--config", configPath, "--data-dir", filepath.Join(tmpDir, "data")})

	if code != 1 {
		t.Errorf("expected exit code 1 with invalid config, got %d", code)
	}
}

func TestGenerateHubConfig(t *testing.T) {
	configJSON, err := GenerateHubConfig(8420, 1883, "./data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if configJSON == "" {
		t.Error("expected non-empty config JSON")
	}

	// Verify it contains expected fields
	if !contains(configJSON, "8420") {
		t.Error("config should contain port 8420")
	}
	if !contains(configJSON, "1883") {
		t.Error("config should contain MQTT port 1883")
	}
}

func TestCheckPort_NotListening(t *testing.T) {
	// Port 59999 should not be in use
	if checkPort(59999) {
		t.Error("expected port 59999 to not be listening")
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	if ip == "" {
		t.Error("expected non-empty IP")
	}
	// Should return an IP or the fallback string
	if ip != "YOUR_SERVER_IP" && len(ip) < 7 {
		t.Errorf("unexpected IP format: %s", ip)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
