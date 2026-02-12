# Config Backup & Hardware Recovery

**Problem:** When hardware fails (laptop dies, Pi corrupts, server crash), how do you recover your agent configuration?

**Solution:** Automated config backup to Turso cloud database.

## Quick Start

### 1. Set Up Turso Credentials

```bash
# Export your Turso database credentials
export EVOCLAW_TURSO_URL="libsql://your-database.turso.io"
export EVOCLAW_TURSO_TOKEN="eyJhbGc..."

# Optional: Add to your shell profile
echo 'export EVOCLAW_TURSO_URL="..."' >> ~/.bashrc
echo 'export EVOCLAW_TURSO_TOKEN="..."' >> ~/.bashrc
```

### 2. Backup Your Config

```bash
# Manual backup
./scripts/backup-config.sh evoclaw.json

# Output:
# ✅ Config backed up to Turso
#    Agents: my-agent-1,my-agent-2
#    Device: fb988b23...
```

### 3. Automate Backups

Add to your startup script or systemd service:

```bash
#!/bin/bash
# startup.sh

# Backup config before starting
./scripts/backup-config.sh evoclaw.json

# Start agent
./evoclaw --config evoclaw.json
```

### 4. Recover After Hardware Failure

On your new hardware:

```bash
# Set up credentials (same as step 1)
export EVOCLAW_TURSO_URL="..."
export EVOCLAW_TURSO_TOKEN="..."

# Run recovery tool
./scripts/restore-config.sh

# Interactive prompt:
# [1] my-agent-1,my-agent-2 - Device: fb988b23... - 1770866181 - Auto-backup
# [2] my-agent-1,my-agent-2 - Device: fb988b23... - 1770866295 - Auto-backup
# Enter backup ID: 2

# ✅ Config restored to: evoclaw-restored.json

# Review and use
cat evoclaw-restored.json
mv evoclaw-restored.json evoclaw.json
./evoclaw --config evoclaw.json
```

## How It Works

### Backup Process

1. **Reads config file** - Extracts agent IDs and full configuration
2. **Generates device ID** - Creates unique hardware identifier (stored in `.evoclaw-device-id`)
3. **Encodes config** - Base64 encoding to safely store JSON in Turso
4. **Uploads to Turso** - Stores in `config_backups` table with timestamp

### Database Schema

```sql
CREATE TABLE config_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,        -- Comma-separated: "agent1,agent2"
    device_id TEXT,                -- UUID of physical device
    config_json TEXT NOT NULL,     -- Base64-encoded configuration
    created_at TEXT NOT NULL,      -- Unix timestamp
    notes TEXT                     -- "Auto-backup" or custom note
);
```

### Recovery Process

1. **Lists all backups** - Shows ID, agents, device, timestamp
2. **User selects backup** - Enter ID of the backup to restore
3. **Downloads and decodes** - Fetches base64 config and decodes to JSON
4. **Saves locally** - Writes to `evoclaw-restored.json`

## Architecture

### Shared Database Approach

The backup system uses a **shared Turso database** for all agents:

```
┌─────────────────────────────────┐
│  Turso Database (Cloud)         │
├─────────────────────────────────┤
│  config_backups table           │
│  ├─ Agent A (device1) backup 1  │
│  ├─ Agent A (device1) backup 2  │
│  ├─ Agent B (device2) backup 1  │
│  └─ Agent B (device2) backup 2  │
└─────────────────────────────────┘
         ↑              ↑
    [Device 1]    [Device 2]
```

**Benefits:**
- ✅ Single database for all agents
- ✅ Easy recovery (just need Turso credentials)
- ✅ Version history (all backups kept)
- ✅ Cost-effective (one database subscription)

**Data Isolation:**
Each backup has:
- Unique `id` (auto-increment)
- `device_id` (identifies hardware)
- `agent_id` (comma-separated list)
- Timestamp for chronological ordering

### Agent Memory vs Config Backup

**Separate systems with different purposes:**

| Feature | Agent Memory | Config Backup |
|---------|-------------|---------------|
| **Purpose** | Long-term facts, context | Hardware recovery |
| **Tables** | `cold_memory`, `warm_memory`, `hot_state` | `config_backups` |
| **Data Isolation** | `agent_id` column | `device_id` + `agent_id` |
| **Update Frequency** | Continuous (every interaction) | On startup / manual |
| **Size** | Growing (facts accumulate) | Fixed (config size) |

When you restore a config with the same `agent_id`, the agent automatically reconnects to its memory in the shared memory tables.

## Recovery Scenarios

### Scenario 1: Laptop Dies

**Problem:** Hardware failure, need to move agent to new laptop

**Solution:**
1. On new laptop, install EvoClaw
2. Set Turso credentials
3. Run `./scripts/restore-config.sh`
4. Select latest backup
5. Start agent with restored config

**Data Loss:** None - config restored, memory already in Turso

### Scenario 2: Config Corrupted

**Problem:** Edited config, broke JSON syntax, can't start

**Solution:**
1. Run `./scripts/restore-config.sh`
2. Select backup from before corruption
3. Replace broken config with restored version

### Scenario 3: Testing Changes

**Problem:** Want to experiment without losing working config

**Solution:**
1. Make changes (current config already backed up on last startup)
2. Test changes
3. If broken, restore previous backup

### Scenario 4: Multi-Device Setup

**Problem:** Running same agent on multiple devices (dev laptop + prod server)

**Solution:**
1. Backup on device 1
2. Restore on device 2
3. Each device gets unique `device_id` but shares agent config

## Advanced Usage

### Custom Backup Notes

```bash
# Backup with custom note
./scripts/backup-config.sh evoclaw.json

# Then manually add note to database:
# UPDATE config_backups SET notes='Before experiment' WHERE id=X;
```

### List Backups via CLI

```bash
curl -s "$EVOCLAW_TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $EVOCLAW_TURSO_TOKEN" \
  -d '{"requests": [{"type": "execute", "stmt": {"sql": "SELECT id, agent_id, device_id, datetime(created_at, \"unixepoch\") as time FROM config_backups ORDER BY id DESC LIMIT 10"}}]}' \
  | jq '.results[0].response.result.rows'
```

### Cleanup Old Backups

```bash
# Keep only last 30 days
curl -s "$EVOCLAW_TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $EVOCLAW_TURSO_TOKEN" \
  -d '{"requests": [{"type": "execute", "stmt": {"sql": "DELETE FROM config_backups WHERE cast(created_at as integer) < strftime(\"%s\", \"now\", \"-30 days\")"}}]}'

# Keep only last 50 backups
curl -s "$EVOCLAW_TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $EVOCLAW_TURSO_TOKEN" \
  -d '{"requests": [{"type": "execute", "stmt": {"sql": "DELETE FROM config_backups WHERE id NOT IN (SELECT id FROM config_backups ORDER BY id DESC LIMIT 50)"}}]}'
```

### Backup to Multiple Destinations

For extra safety, backup to multiple places:

```bash
# 1. Turso (cloud)
./scripts/backup-config.sh evoclaw.json

# 2. Git
git add evoclaw.json && git commit -m "Config backup $(date)"

# 3. USB stick
cp evoclaw.json /media/usb/evoclaw-backup-$(date +%Y%m%d).json

# 4. Email to yourself
cat evoclaw.json | mail -s "EvoClaw Config Backup" you@example.com
```

## Security Considerations

### Token Storage

**Current:** Tokens are stored in environment variables

**Alternative (More Secure):**
```bash
# Encrypt credentials file
openssl enc -aes-256-cbc -salt -in credentials.txt -out credentials.enc
# Decrypt when needed
export EVOCLAW_TURSO_TOKEN=$(openssl enc -aes-256-cbc -d -in credentials.enc)
```

### Access Control

Anyone with your Turso credentials can:
- ✅ Backup configs (no harm)
- ⚠️ Restore configs (reveals agent configuration)
- ⚠️ Read agent IDs and device IDs

**Mitigation:**
- Store credentials securely (not in public repos)
- Use separate Turso database for sensitive agents
- Rotate tokens periodically

### Config Contents

Config files may contain:
- API keys (model providers)
- Database credentials
- Private keys (blockchain wallets)

**Recommendation:**
- Use `ENV:` prefix for sensitive values in config
- Store actual secrets in environment variables
- Don't hardcode credentials in config files

Example:
```json
{
  "apiKey": "ENV:ANTHROPIC_API_KEY",  // ✅ Reference env var
  "apiKey": "sk-ant-actual-key"       // ❌ Don't do this
}
```

## Troubleshooting

### "Missing Turso credentials"

**Problem:** Script can't find `EVOCLAW_TURSO_URL` or `EVOCLAW_TURSO_TOKEN`

**Solution:**
```bash
# Check if set
echo $EVOCLAW_TURSO_URL
echo $EVOCLAW_TURSO_TOKEN

# Set them
export EVOCLAW_TURSO_URL="..."
export EVOCLAW_TURSO_TOKEN="..."
```

### "jq not found"

**Problem:** `jq` command is missing (needed for JSON parsing)

**Solution:**
```bash
# Ubuntu/Debian
sudo apt-get install jq

# macOS
brew install jq

# Alpine
apk add jq
```

### "No backups found"

**Problem:** Database has no backups yet

**Solution:**
```bash
# Create first backup
./scripts/backup-config.sh evoclaw.json

# Verify
./scripts/restore-config.sh
```

### "Backup may have failed"

**Problem:** Network issue or invalid credentials

**Solution:**
1. Check Turso dashboard - is database accessible?
2. Verify credentials are correct
3. Check network connectivity
4. Look for error details in script output

## Best Practices

### For Human Users (2-10 agents)

✅ **Recommended:**
- Automated backup on every startup
- Keep all backups (version history)
- Single shared Turso database

### For Enterprise (100+ agents)

✅ **Recommended:**
- Automated backup + cleanup policy (keep last 90 days)
- Multiple Turso databases by environment (dev/staging/prod)
- Backup to git repositories as well
- Monitoring/alerting for failed backups

## Files

- `scripts/backup-config.sh` - Backup automation
- `scripts/restore-config.sh` - Recovery tool
- `docs/CONFIG-BACKUP.md` - This documentation

## Summary

**Problem:** Hardware failure = lost agent configuration  
**Solution:** Automated cloud backup with easy recovery

**Key Features:**
- ✅ Automated - Backup on every startup
- ✅ Cloud storage - Survives hardware failure
- ✅ Version history - All backups kept
- ✅ Easy recovery - Interactive restore tool
- ✅ Multi-agent - One database for all agents
- ✅ Secure - Credentials in env vars

**One-Time Setup:**
```bash
export EVOCLAW_TURSO_URL="..."
export EVOCLAW_TURSO_TOKEN="..."
```

**Daily Use:**
```bash
./scripts/backup-config.sh evoclaw.json  # Backup
./evoclaw --config evoclaw.json          # Start agent
```

**After Hardware Failure:**
```bash
./scripts/restore-config.sh  # Recover
```

That's it! Your agent configs are now disaster-proof. 🎉

---

**Version:** 1.0  
**Last Updated:** 2026-02-12  
**Tested On:** Ubuntu 22.04, macOS 14, Alpine Linux
