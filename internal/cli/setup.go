package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/clawinfra/evoclaw/internal/config"
)

// SetupCLI handles the `evoclaw setup` subcommands.
type SetupCLI struct {
	stdout *os.File
}

// NewSetupCLI creates a new setup CLI handler.
func NewSetupCLI() *SetupCLI {
	return &SetupCLI{stdout: os.Stdout}
}

// NewSetupCLIWithOutput creates a SetupCLI with custom output (for testing).
func NewSetupCLIWithOutput(out *os.File) *SetupCLI {
	return &SetupCLI{stdout: out}
}

// Run executes the setup subcommand based on args.
// Returns exit code.
func (s *SetupCLI) Run(args []string) int {
	if len(args) == 0 {
		s.printUsage()
		return 1
	}

	switch args[0] {
	case "hub":
		return s.runHub(args[1:])
	case "help", "--help", "-h":
		s.printUsage()
		return 0
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown setup command: %s\n", args[0])
		s.printUsage()
		return 1
	}
}

// printUsage displays setup subcommand help.
func (s *SetupCLI) printUsage() {
	_, _ = fmt.Fprintln(s.stdout, `Usage: evoclaw setup <command> [options]

Initialize EvoClaw deployment components.

Commands:
  hub       Set up this machine as an EvoClaw hub (orchestrator + MQTT)

Examples:
  evoclaw setup hub
  evoclaw setup hub --port 8420 --mqtt-port 1883`)
}

// runHub handles `evoclaw setup hub`.
func (s *SetupCLI) runHub(args []string) int {
	fs := flag.NewFlagSet("setup hub", flag.ContinueOnError)
	port := fs.Int("port", 8420, "API port")
	mqttPort := fs.Int("mqtt-port", 1883, "MQTT broker port")
	dataDir := fs.String("data-dir", "./data", "Data directory")
	configPath := fs.String("config", "evoclaw.json", "Config file path")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Step 1: Generate config if it doesn't exist
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(s.stdout, "📝 Generating config at %s\n", *configPath)
		cfg := config.DefaultConfig()
		cfg.Server.Port = *port
		cfg.Server.DataDir = *dataDir
		cfg.MQTT.Port = *mqttPort
		cfg.MQTT.Host = "0.0.0.0"

		if err := cfg.Save(*configPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "❌ Failed to save config: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(s.stdout, "   ✓ Config written\n")
	} else {
		_, _ = fmt.Fprintf(s.stdout, "📝 Config already exists at %s\n", *configPath)
		// Verify it's valid
		if _, err := config.Load(*configPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "❌ Config is invalid: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(s.stdout, "   ✓ Config valid\n")
	}

	// Step 2: Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0750); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "❌ Failed to create data directory: %v\n", err)
		return 1
	}

	// Step 3: Check if MQTT broker is running
	mqttRunning := checkPort(*mqttPort)
	if mqttRunning {
		_, _ = fmt.Fprintf(s.stdout, "🔌 MQTT broker detected on port %d ✓\n", *mqttPort)
	} else {
		_, _ = fmt.Fprintf(s.stdout, "🔌 MQTT broker not detected on port %d\n", *mqttPort)
		started := s.tryStartMQTT(*mqttPort)
		if started {
			_, _ = fmt.Fprintf(s.stdout, "   ✓ MQTT broker started\n")
		} else {
			_, _ = fmt.Fprintf(s.stdout, "   ⚠ Start an MQTT broker manually (e.g., mosquitto)\n")
		}
	}

	// Step 4: Detect local IP for join command
	localIP := getLocalIP()

	// Step 5: Print success banner
	_, _ = fmt.Fprintln(s.stdout)
	_, _ = fmt.Fprintln(s.stdout, "🧬 EvoClaw Hub Ready!")
	_, _ = fmt.Fprintln(s.stdout)
	_, _ = fmt.Fprintf(s.stdout, "  API:       http://0.0.0.0:%d\n", *port)
	_, _ = fmt.Fprintf(s.stdout, "  MQTT:      0.0.0.0:%d\n", *mqttPort)
	_, _ = fmt.Fprintf(s.stdout, "  Dashboard: http://localhost:%d\n", *port)
	_, _ = fmt.Fprintln(s.stdout)
	_, _ = fmt.Fprintln(s.stdout, "  To add an edge agent, run this on your device:")
	joinCmd := fmt.Sprintf("  evoclaw-agent join %s", localIP)
	border := strings.Repeat("─", len(joinCmd)+2)
	_, _ = fmt.Fprintf(s.stdout, "  ┌%s┐\n", border)
	_, _ = fmt.Fprintf(s.stdout, "  │ %s │\n", joinCmd)
	_, _ = fmt.Fprintf(s.stdout, "  └%s┘\n", border)
	_, _ = fmt.Fprintln(s.stdout)

	return 0
}

// tryStartMQTT attempts to start an MQTT broker using Podman or Docker.
func (s *SetupCLI) tryStartMQTT(port int) bool {
	// Try Podman first, then Docker
	for _, runtime := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(runtime); err != nil {
			continue
		}

		_, _ = fmt.Fprintf(s.stdout, "   Starting MQTT broker via %s...\n", runtime)
		// #nosec G204 — runtime and port are controlled
		cmd := exec.Command(runtime, "run", "-d",
			"--name", "evoclaw-mqtt",
			"-p", fmt.Sprintf("%d:1883", port),
			"docker.io/library/eclipse-mosquitto:2",
			"mosquitto", "-c", "/mosquitto-no-auth.conf",
		)

		if output, err := cmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(s.stdout, "   ⚠ Failed to start via %s: %s\n", runtime, strings.TrimSpace(string(output)))
			continue
		}
		return true
	}
	return false
}

// checkPort returns true if something is listening on the given TCP port.
func checkPort(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1e9) // 1 second
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// getLocalIP returns the preferred outbound local IP address.
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "YOUR_SERVER_IP"
	}
	defer func() { _ = conn.Close() }()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// GenerateHubConfig creates and returns a default hub config as JSON string (for testing).
func GenerateHubConfig(port, mqttPort int, dataDir string) (string, error) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = port
	cfg.Server.DataDir = dataDir
	cfg.MQTT.Port = mqttPort
	cfg.MQTT.Host = "0.0.0.0"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(data), nil
}
