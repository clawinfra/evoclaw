package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const systemdUnitTemplate = `[Unit]
Description=EvoClaw Self-Evolving Agent Framework
Documentation=https://github.com/clawinfra/evoclaw
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User={{.User}}
Group={{.Group}}
WorkingDirectory={{.WorkDir}}
ExecStart={{.ExecPath}} --config {{.ConfigPath}}
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=evoclaw

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths={{.DataDir}}

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
`

type systemdConfig struct {
	User       string
	Group      string
	WorkDir    string
	ExecPath   string
	ConfigPath string
	DataDir    string
}

func installSystemd() error {
	fmt.Println("📦 Installing systemd service...")

	// Get current user
	user := os.Getenv("USER")
	if user == "" {
		user = "evoclaw"
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	execPath, _ = filepath.Abs(execPath)

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Default config and data paths
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(workDir, "evoclaw.json")
	dataDir := filepath.Join(home, ".evoclaw")

	// Check if config exists
	if !fileExists(configPath) {
		// Try user home
		altConfig := filepath.Join(dataDir, "evoclaw.json")
		if fileExists(altConfig) {
			configPath = altConfig
		}
	}

	cfg := systemdConfig{
		User:       user,
		Group:      user,
		WorkDir:    workDir,
		ExecPath:   execPath,
		ConfigPath: configPath,
		DataDir:    dataDir,
	}

	// Generate unit file
	tmpl, err := template.New("systemd").Parse(systemdUnitTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Determine if user or system service
	isRoot := os.Geteuid() == 0
	var unitPath string

	if isRoot {
		// System-wide service
		unitPath = "/etc/systemd/system/evoclaw.service"
	} else {
		// User service
		unitDir := filepath.Join(home, ".config", "systemd", "user")
		os.MkdirAll(unitDir, 0755)
		unitPath = filepath.Join(unitDir, "evoclaw.service")
	}

	// Write unit file
	f, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("create unit file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	fmt.Printf("✅ Systemd unit installed: %s\n", unitPath)

	// Reload systemd
	var reloadCmd *exec.Cmd
	if isRoot {
		reloadCmd = exec.Command("systemctl", "daemon-reload")
	} else {
		reloadCmd = exec.Command("systemctl", "--user", "daemon-reload")
	}

	if err := reloadCmd.Run(); err != nil {
		fmt.Printf("⚠️  Warning: systemctl daemon-reload failed: %v\n", err)
	}

	// Print usage instructions
	fmt.Println("\n📋 Next steps:")
	if isRoot {
		fmt.Println("   sudo systemctl enable evoclaw")
		fmt.Println("   sudo systemctl start evoclaw")
		fmt.Println("   sudo systemctl status evoclaw")
	} else {
		fmt.Println("   systemctl --user enable evoclaw")
		fmt.Println("   systemctl --user start evoclaw")
		fmt.Println("   systemctl --user status evoclaw")
	}

	return nil
}

func uninstallSystemd() error {
	fmt.Println("🗑️  Uninstalling systemd service...")

	isRoot := os.Geteuid() == 0
	var unitPath string

	if isRoot {
		unitPath = "/etc/systemd/system/evoclaw.service"
	} else {
		home, _ := os.UserHomeDir()
		unitPath = filepath.Join(home, ".config", "systemd", "user", "evoclaw.service")
	}

	// Stop service first
	var stopCmd *exec.Cmd
	if isRoot {
		stopCmd = exec.Command("systemctl", "stop", "evoclaw")
		exec.Command("systemctl", "disable", "evoclaw").Run()
	} else {
		stopCmd = exec.Command("systemctl", "--user", "stop", "evoclaw")
		exec.Command("systemctl", "--user", "disable", "evoclaw").Run()
	}
	stopCmd.Run() // Ignore errors

	// Remove unit file
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file: %w", err)
	}

	// Reload systemd
	var reloadCmd *exec.Cmd
	if isRoot {
		reloadCmd = exec.Command("systemctl", "daemon-reload")
	} else {
		reloadCmd = exec.Command("systemctl", "--user", "daemon-reload")
	}
	reloadCmd.Run()

	fmt.Println("✅ Systemd service uninstalled")
	return nil
}
