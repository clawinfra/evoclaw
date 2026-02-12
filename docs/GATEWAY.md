# Gateway / Daemon Mode

> *Run EvoClaw as a background service with automatic restart and graceful shutdown*

---

## Overview

The EvoClaw Gateway provides daemon/service management for running EvoClaw as a background process with:

- **Systemd integration** (Linux)
- **Launchd integration** (macOS)
- **Graceful shutdown** (SIGTERM handling)
- **Auto-restart** on crash
- **Signal-based control** (reload, update)
- **PID file management**

---

## Quick Start

### Linux (systemd)

```bash
# Install service
evoclaw gateway install

# Enable and start
sudo systemctl enable evoclaw
sudo systemctl start evoclaw

# Check status
sudo systemctl status evoclaw

# View logs
sudo journalctl -u evoclaw -f

# Restart
sudo systemctl restart evoclaw

# Stop
sudo systemctl stop evoclaw
```

### macOS (launchd)

```bash
# Install service
evoclaw gateway install

# Start service
launchctl start com.clawinfra.evoclaw

# Check status
launchctl list | grep evoclaw

# View logs
tail -f ~/.evoclaw/logs/evoclaw.log

# Stop service
launchctl stop com.clawinfra.evoclaw

# Restart
launchctl unload ~/Library/LaunchAgents/com.clawinfra.evoclaw.plist
launchctl load ~/Library/LaunchAgents/com.clawinfra.evoclaw.plist
```

---

## Commands

### install

Install systemd/launchd service files.

```bash
evoclaw gateway install
```

**Linux:** Creates `/etc/systemd/system/evoclaw.service` (root) or `~/.config/systemd/user/evoclaw.service` (user)

**macOS:** Creates `/Library/LaunchDaemons/com.clawinfra.evoclaw.plist` (root) or `~/Library/LaunchAgents/com.clawinfra.evoclaw.plist` (user)

**What it does:**
- Detects OS (Linux/macOS)
- Finds executable path
- Finds config file
- Generates service file from template
- Reloads service manager
- Prints usage instructions

### uninstall

Remove service files and stop daemon.

```bash
evoclaw gateway uninstall
```

**What it does:**
- Stops running service
- Disables auto-start
- Removes service file
- Reloads service manager

### start

Start EvoClaw in daemon mode.

```bash
evoclaw gateway start
```

**What it does:**
- Checks if already running
- Starts EvoClaw in background
- Creates PID file
- Detaches from terminal

**Note:** For production use, prefer `systemctl start` or `launchctl start` after install.

### stop

Stop running EvoClaw daemon gracefully.

```bash
evoclaw gateway stop
```

**What it does:**
- Reads PID from file
- Sends SIGTERM for graceful shutdown
- Waits up to 30 seconds
- Force kills if timeout
- Removes PID file

### status

Check if EvoClaw is running.

```bash
evoclaw gateway status
```

**Output:**
```
✅ EvoClaw is running (PID: 12345)
   Process: 12345
   PID file: /home/user/.evoclaw/evoclaw.pid
```

**Exit codes:**
- `0` - Running
- `1` - Not running

### restart

Restart EvoClaw daemon.

```bash
evoclaw gateway restart
```

**What it does:**
- Stops daemon if running
- Waits 2 seconds
- Starts daemon

---

## Service Configuration

### Systemd Unit (Linux)

**Location:**
- System: `/etc/systemd/system/evoclaw.service`
- User: `~/.config/systemd/user/evoclaw.service`

**Template:**
```ini
[Unit]
Description=EvoClaw Self-Evolving Agent Framework
Documentation=https://github.com/clawinfra/evoclaw
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=evoclaw
WorkingDirectory=/path/to/evoclaw
ExecStart=/path/to/evoclaw --config /path/to/config.json
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home/user/.evoclaw

[Install]
WantedBy=multi-user.target
```

**Features:**
- Auto-restart on failure
- 5-second restart delay
- Logs to systemd journal
- Security hardening (NoNewPrivileges, ProtectSystem)
- Network dependency

### Launchd Plist (macOS)

**Location:**
- System: `/Library/LaunchDaemons/com.clawinfra.evoclaw.plist`
- User: `~/Library/LaunchAgents/com.clawinfra.evoclaw.plist`

**Template:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ...>
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.clawinfra.evoclaw</string>
	
	<key>ProgramArguments</key>
	<array>
		<string>/path/to/evoclaw</string>
		<string>--config</string>
		<string>/path/to/config.json</string>
	</array>
	
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
	<string>/path/to/logs/evoclaw.log</string>
	
	<key>ThrottleInterval</key>
	<integer>5</integer>
</dict>
</plist>
```

**Features:**
- Start on boot (RunAtLoad)
- Auto-restart on crash
- 5-second throttle between restarts
- Logs to file

---

## Graceful Shutdown

EvoClaw handles shutdown signals gracefully:

| Signal | Behavior |
|--------|----------|
| `SIGTERM` | Graceful shutdown (save state, stop services) |
| `SIGINT` | Same as SIGTERM (Ctrl+C) |
| `SIGHUP` | Reload config (planned - not yet implemented) |
| `SIGUSR1` | Self-update and restart (planned - not yet implemented) |

**Shutdown sequence:**
1. Receive SIGTERM
2. Stop accepting new requests
3. Save all agent state
4. Save all memory
5. Stop orchestrator
6. Exit cleanly

**Timeout:** 30 seconds (systemd/launchd will force-kill after)

---

## Logs

### Systemd (Linux)

```bash
# View all logs
sudo journalctl -u evoclaw

# Follow logs (live)
sudo journalctl -u evoclaw -f

# Last 100 lines
sudo journalctl -u evoclaw -n 100

# Since boot
sudo journalctl -u evoclaw -b

# Filter by date
sudo journalctl -u evoclaw --since "2026-02-12"
```

### Launchd (macOS)

```bash
# View stdout
tail -f ~/.evoclaw/logs/evoclaw.log

# View stderr
tail -f ~/.evoclaw/logs/evoclaw.error.log

# Both
tail -f ~/.evoclaw/logs/*.log
```

---

## Troubleshooting

### Service won't start

**Check status:**
```bash
# Linux
sudo systemctl status evoclaw

# macOS
launchctl list | grep evoclaw
```

**Check logs:**
```bash
# Linux
sudo journalctl -u evoclaw -n 50

# macOS
tail -50 ~/.evoclaw/logs/evoclaw.error.log
```

**Common issues:**
- Config file not found
- Port already in use
- Permissions error
- Missing dependencies

### Service keeps restarting

**Check restart count:**
```bash
# Linux
systemctl show evoclaw | grep RestartCount

# macOS
launchctl list com.clawinfra.evoclaw
```

**Disable auto-restart temporarily:**
```bash
# Linux
sudo systemctl edit evoclaw
# Add: [Service]
#      Restart=no

# macOS
# Edit plist, set KeepAlive to false
```

### Can't stop service

**Force kill:**
```bash
# Linux
sudo systemctl kill -s SIGKILL evoclaw

# macOS
sudo kill -9 $(pgrep evoclaw)
```

### Logs not appearing

**Check log paths:**
```bash
# Linux (systemd journal)
sudo journalctl -u evoclaw --verify

# macOS
ls -la ~/.evoclaw/logs/
```

---

## Security

### User Isolation

Run as dedicated user (recommended):

```bash
# Create evoclaw user
sudo useradd -r -s /bin/false evoclaw

# Create directories
sudo mkdir -p /var/lib/evoclaw
sudo chown evoclaw:evoclaw /var/lib/evoclaw

# Update service to use evoclaw user
sudo systemctl edit evoclaw
# Add: [Service]
#      User=evoclaw
#      Group=evoclaw
```

### Systemd Hardening

Already applied in generated unit:
- `NoNewPrivileges=true` - Prevent privilege escalation
- `PrivateTmp=true` - Private /tmp
- `ProtectSystem=strict` - Read-only system directories
- `ProtectHome=read-only` - Read-only home (except data dir)

Additional hardening options:
- `ProtectKernelModules=true`
- `ProtectKernelTunables=true`
- `ProtectControlGroups=true`
- `RestrictRealtime=true`
- `LockPersonality=true`

### File Permissions

```bash
# Config file (sensitive)
chmod 600 evoclaw.json
chown evoclaw:evoclaw evoclaw.json

# Data directory
chmod 750 ~/.evoclaw
chown -R evoclaw:evoclaw ~/.evoclaw

# PID file
chmod 644 ~/.evoclaw/evoclaw.pid
```

---

## Development

### Testing Service Files

**Validate systemd unit:**
```bash
systemd-analyze verify evoclaw.service
```

**Validate launchd plist:**
```bash
plutil -lint com.clawinfra.evoclaw.plist
```

### Manual Daemon Mode

For testing without service manager:

```bash
# Start in background
nohup evoclaw --config evoclaw.json > evoclaw.log 2>&1 &
echo $! > evoclaw.pid

# Stop
kill $(cat evoclaw.pid)
```

---

## Future Enhancements

- [ ] SIGHUP config reload (hot reload without restart)
- [ ] SIGUSR1 self-update (download and restart with new binary)
- [ ] `evoclaw gateway logs` command (tail logs cross-platform)
- [ ] `evoclaw gateway health` (HTTP health check endpoint)
- [ ] Windows service support (via nssm or sc.exe)
- [ ] Docker/Podman health checks
- [ ] Prometheus metrics endpoint

---

**Status:** Production Ready ✅  
**Added:** 2026-02-12  
**Platforms:** Linux (systemd), macOS (launchd)

---

*Run EvoClaw like a pro. Install once, run forever.* 🧬
