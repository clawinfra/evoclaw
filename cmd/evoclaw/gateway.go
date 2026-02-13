package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Gateway commands for daemon management
func runGatewayCommand(args []string) error {
	if len(args) < 1 {
		printGatewayHelp()
		return fmt.Errorf("gateway command required")
	}

	cmd := args[0]
	
	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		printGatewayHelp()
		return nil
	}

	switch cmd {
	case "start":
		return gatewayStart()
	case "stop":
		return gatewayStop()
	case "status":
		return gatewayStatus()
	case "restart":
		return gatewayRestart()
	case "install":
		return gatewayInstall()
	case "uninstall":
		return gatewayUninstall()
	default:
		return fmt.Errorf("unknown gateway command: %s", cmd)
	}
}

func gatewayStart() error {
	pidFile := getPIDFile()

	// Check if already running
	if pid, running := checkRunning(); running {
		return fmt.Errorf("EvoClaw is already running (PID: %d)", pid)
	}

	fmt.Println("🧬 Starting EvoClaw daemon...")

	// Start in background
	if err := daemonize(); err != nil {
		return fmt.Errorf("failed to daemonize: %w", err)
	}

	fmt.Println("✅ EvoClaw daemon started")
	fmt.Printf("   PID file: %s\n", pidFile)
	fmt.Println("   Check logs: evoclaw gateway logs")
	fmt.Println("   Status: evoclaw gateway status")

	return nil
}

func gatewayStop() error {
	pid, running := checkRunning()
	if !running {
		fmt.Println("EvoClaw is not running")
		return nil
	}

	fmt.Printf("🛑 Stopping EvoClaw daemon (PID: %d)...\n", pid)

	// Send SIGTERM for graceful shutdown
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait up to 30 seconds for graceful shutdown
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		if _, running := checkRunning(); !running {
			fmt.Println("✅ EvoClaw stopped gracefully")
			os.Remove(getPIDFile())
			return nil
		}
	}

	// Force kill if not stopped
	fmt.Println("⚠️  Graceful shutdown timeout, forcing...")
	if err := process.Kill(); err != nil {
		return fmt.Errorf("force kill: %w", err)
	}

	os.Remove(getPIDFile())
	fmt.Println("✅ EvoClaw stopped (forced)")
	return nil
}

func gatewayStatus() error {
	pid, running := checkRunning()

	if running {
		fmt.Printf("✅ EvoClaw is running (PID: %d)\n", pid)
		
		// Try to get uptime
		process, _ := os.FindProcess(pid)
		if process != nil {
			fmt.Printf("   Process: %d\n", pid)
			fmt.Printf("   PID file: %s\n", getPIDFile())
		}
		
		return nil
	}

	fmt.Println("❌ EvoClaw is not running")
	return fmt.Errorf("not running")
}

func gatewayRestart() error {
	fmt.Println("🔄 Restarting EvoClaw daemon...")
	
	// Stop if running
	if _, running := checkRunning(); running {
		if err := gatewayStop(); err != nil {
			fmt.Printf("Warning: stop failed: %v\n", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Start
	return gatewayStart()
}

func gatewayInstall() error {
	// Detect OS and install appropriate service file
	switch {
	case fileExists("/etc/systemd/system"):
		return installSystemd()
	case fileExists("/Library/LaunchDaemons"):
		return installLaunchd()
	default:
		return fmt.Errorf("unsupported init system (need systemd or launchd)")
	}
}

func gatewayUninstall() error {
	switch {
	case fileExists("/etc/systemd/system"):
		return uninstallSystemd()
	case fileExists("/Library/LaunchDaemons"):
		return uninstallLaunchd()
	default:
		return fmt.Errorf("unsupported init system")
	}
}

// Helper functions

func checkRunning() (int, bool) {
	pidFile := getPIDFile()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0, false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}

	// Send signal 0 to check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}

	return pid, true
}

func getPIDFile() string {
	// Try user-specific location first
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".evoclaw", "evoclaw.pid")
	}
	// Fallback to /var/run
	return "/var/run/evoclaw.pid"
}

func daemonize() error {
	// Fork and exit parent (Unix daemonization pattern)
	// Note: Go doesn't support traditional fork(), so we use exec
	
	// For now, just run in background with proper signal handling
	// A proper implementation would use a process manager
	
	return fmt.Errorf("daemonize not yet implemented - use systemd/launchd install instead")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func printGatewayHelp() {
	fmt.Println(`EvoClaw Gateway - Daemon Management

USAGE:
    evoclaw gateway <command>

COMMANDS:
    start       Start EvoClaw daemon
    stop        Stop EvoClaw daemon gracefully
    status      Check if EvoClaw is running
    restart     Restart EvoClaw daemon
    install     Install systemd/launchd service
    uninstall   Remove systemd/launchd service
    help        Show this help message

EXAMPLES:
    # Start daemon
    evoclaw gateway start

    # Check status
    evoclaw gateway status

    # Install service (Linux/macOS)
    evoclaw gateway install

    # Use systemd (after install)
    sudo systemctl start evoclaw
    sudo systemctl status evoclaw

    # Use launchd (after install)
    launchctl start com.clawinfra.evoclaw

For more information, see: docs/GATEWAY.md`)
}
