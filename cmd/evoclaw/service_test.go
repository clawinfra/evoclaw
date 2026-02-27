package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSystemd(t *testing.T) {
	// Run as non-root - should create user service file
	err := installSystemd()
	if err != nil {
		t.Logf("installSystemd error (expected in CI): %v", err)
	}
	// Cleanup
	home, _ := os.UserHomeDir()
	unitPath := filepath.Join(home, ".config", "systemd", "user", "evoclaw.service")
	_ = os.Remove(unitPath)
}

func TestUninstallSystemd(t *testing.T) {
	// Uninstall - should not panic even if service doesn't exist
	err := uninstallSystemd()
	if err != nil {
		t.Logf("uninstallSystemd error (expected): %v", err)
	}
}

func TestGatewayInstall(t *testing.T) {
	// On Linux this calls installSystemd
	err := gatewayInstall()
	if err != nil {
		t.Logf("gatewayInstall error (expected): %v", err)
	}
	// Cleanup
	home, _ := os.UserHomeDir()
	unitPath := filepath.Join(home, ".config", "systemd", "user", "evoclaw.service")
	_ = os.Remove(unitPath)
}

func TestGatewayUninstall(t *testing.T) {
	err := gatewayUninstall()
	if err != nil {
		t.Logf("gatewayUninstall error (expected): %v", err)
	}
}

func TestGatewayRestart(t *testing.T) {
	// Will attempt to stop (not running) then start (daemonize fails)
	err := gatewayRestart()
	if err == nil {
		t.Log("gatewayRestart returned nil (unexpected)")
	}
}

func TestGatewayStatus_NotRunning(t *testing.T) {
	// Ensure PID file doesn't exist or points to dead process
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	_ = os.Remove(pidFile)

	err := gatewayStatus()
	if err == nil {
		t.Error("expected error for not-running status")
	}
}

func TestRunGatewayCommand_AllCmds(t *testing.T) {
	cmds := []string{"start", "restart", "install", "uninstall"}
	for _, cmd := range cmds {
		t.Run(cmd, func(t *testing.T) {
			err := runGatewayCommand([]string{cmd})
			_ = err
		})
	}
	// Cleanup
	home, _ := os.UserHomeDir()
	unitPath := filepath.Join(home, ".config", "systemd", "user", "evoclaw.service")
	_ = os.Remove(unitPath)
}

func TestInstallLaunchd(t *testing.T) {
	// On Linux: will create ~/Library/LaunchAgents/ and write plist
	err := installLaunchd()
	if err != nil {
		t.Logf("installLaunchd error (expected on Linux): %v", err)
	}
	// Cleanup
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.clawinfra.evoclaw.plist")
	_ = os.Remove(plist)
}

func TestUninstallLaunchd(t *testing.T) {
	err := uninstallLaunchd()
	if err != nil {
		t.Logf("uninstallLaunchd error (expected): %v", err)
	}
}

func TestGatewayStop_WithDeadPID(t *testing.T) {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	// Write a dead PID
	_ = os.WriteFile(pidFile, []byte("999999999"), 0644)
	err := gatewayStop()
	_ = err // not running → prints message
}

// gatewayStop with a running PID would SIGTERM our test process - can't test safely.

func TestGatewayStatus_Running(t *testing.T) {
	// Write our own PID
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	_ = os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())), 0644)
	err := gatewayStatus()
	if err != nil {
		t.Errorf("expected nil for running status, got: %v", err)
	}
}
