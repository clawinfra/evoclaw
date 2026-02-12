package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	
	<key>ProgramArguments</key>
	<array>
		<string>{{.ExecPath}}</string>
		<string>--config</string>
		<string>{{.ConfigPath}}</string>
	</array>
	
	<key>WorkingDirectory</key>
	<string>{{.WorkDir}}</string>
	
	<key>RunAtLoad</key>
	<true/>
	
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
		<key>Crashed</key>
		<true/>
	</dict>
	
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/evoclaw.log</string>
	
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/evoclaw.error.log</string>
	
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
	</dict>
	
	<key>ProcessType</key>
	<string>Background</string>
	
	<key>Nice</key>
	<integer>0</integer>
	
	<key>ThrottleInterval</key>
	<integer>5</integer>
</dict>
</plist>
`

type launchdConfig struct {
	Label      string
	ExecPath   string
	ConfigPath string
	WorkDir    string
	LogDir     string
}

func installLaunchd() error {
	fmt.Println("📦 Installing launchd service...")

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

	// Default paths
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(workDir, "evoclaw.json")
	logDir := filepath.Join(home, ".evoclaw", "logs")

	// Check if config exists
	if !fileExists(configPath) {
		altConfig := filepath.Join(home, ".evoclaw", "evoclaw.json")
		if fileExists(altConfig) {
			configPath = altConfig
		}
	}

	// Create log directory
	os.MkdirAll(logDir, 0755)

	cfg := launchdConfig{
		Label:      "com.clawinfra.evoclaw",
		ExecPath:   execPath,
		ConfigPath: configPath,
		WorkDir:    workDir,
		LogDir:     logDir,
	}

	// Generate plist
	tmpl, err := template.New("launchd").Parse(launchdPlistTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Determine plist location
	isRoot := os.Geteuid() == 0
	var plistPath string

	if isRoot {
		// System-wide daemon
		plistPath = "/Library/LaunchDaemons/com.clawinfra.evoclaw.plist"
	} else {
		// User agent
		plistPath = filepath.Join(home, "Library", "LaunchAgents", "com.clawinfra.evoclaw.plist")
		os.MkdirAll(filepath.Dir(plistPath), 0755)
	}

	// Write plist
	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("create plist: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	fmt.Printf("✅ Launchd plist installed: %s\n", plistPath)

	// Load the service
	var loadCmd *exec.Cmd
	if isRoot {
		loadCmd = exec.Command("launchctl", "load", plistPath)
	} else {
		loadCmd = exec.Command("launchctl", "load", plistPath)
	}

	if err := loadCmd.Run(); err != nil {
		fmt.Printf("⚠️  Warning: launchctl load failed: %v\n", err)
		fmt.Println("   You may need to load it manually:")
		fmt.Printf("   launchctl load %s\n", plistPath)
	} else {
		fmt.Println("✅ Service loaded and will start on boot")
	}

	// Print usage instructions
	fmt.Println("\n📋 Management commands:")
	if isRoot {
		fmt.Println("   sudo launchctl start com.clawinfra.evoclaw")
		fmt.Println("   sudo launchctl stop com.clawinfra.evoclaw")
		fmt.Println("   sudo launchctl unload " + plistPath)
	} else {
		fmt.Println("   launchctl start com.clawinfra.evoclaw")
		fmt.Println("   launchctl stop com.clawinfra.evoclaw")
		fmt.Println("   launchctl unload " + plistPath)
	}
	fmt.Printf("\n📁 Logs: %s\n", logDir)

	return nil
}

func uninstallLaunchd() error {
	fmt.Println("🗑️  Uninstalling launchd service...")

	isRoot := os.Geteuid() == 0
	var plistPath string

	if isRoot {
		plistPath = "/Library/LaunchDaemons/com.clawinfra.evoclaw.plist"
	} else {
		home, _ := os.UserHomeDir()
		plistPath = filepath.Join(home, "Library", "LaunchAgents", "com.clawinfra.evoclaw.plist")
	}

	// Unload service
	unloadCmd := exec.Command("launchctl", "unload", plistPath)
	unloadCmd.Run() // Ignore errors

	// Remove plist
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}

	fmt.Println("✅ Launchd service uninstalled")
	return nil
}
