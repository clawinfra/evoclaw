# ClawChain Auto-Discovery & Registration Design

## Goal
Agents deployed with `--skip-chain` should automatically discover ClawChain mainnet when it launches and register themselves without manual intervention.

---

## Architecture

### 1. Health Check Cron Job
**Frequency:** Every 6 hours (not too aggressive, but responsive within a day)

**What it checks:**
```bash
# Check if ClawChain mainnet is reachable
curl -s https://mainnet.clawchain.win/health
# Expected response: {"status": "ok", "chain": "clawchain", "network": "mainnet"}

# If reachable, check RPC endpoint
curl -s -X POST https://mainnet-rpc.clawchain.win \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"system_health","params":[],"id":1}'
# Expected: {"jsonrpc":"2.0","result":{"isSyncing":false,"peers":...},"id":1}
```

### 2. DID Check Logic
```go
// Pseudocode for the cron job
func CheckAndRegisterClawChain(ctx context.Context) error {
    // 1. Check if ClawChain is already configured
    if cfg.HasChain("clawchain") {
        logger.Info("ClawChain already configured, skipping")
        return nil
    }
    
    // 2. Check if mainnet is reachable
    mainnetURL := "https://mainnet-rpc.clawchain.win"
    if !isClawChainReachable(ctx, mainnetURL) {
        logger.Debug("ClawChain mainnet not yet available")
        return nil // Not an error, just not ready yet
    }
    
    // 3. Check if agent already has a DID (from testnet or elsewhere)
    agentDID := loadAgentDID()
    if agentDID == "" {
        // Generate new DID
        agentDID = generateDID()
    }
    
    // 4. Check if DID is already registered on mainnet
    exists, err := checkDIDExists(ctx, mainnetURL, agentDID)
    if err != nil {
        return fmt.Errorf("check DID: %w", err)
    }
    
    if exists {
        logger.Info("DID already registered on mainnet", "did", agentDID)
        // Just add the adapter to config
        return addClawChainAdapter(mainnetURL, agentDID)
    }
    
    // 5. Register on mainnet
    logger.Info("Registering on ClawChain mainnet", "did", agentDID)
    txHash, err := registerOnClawChain(ctx, mainnetURL, agentDID)
    if err != nil {
        return fmt.Errorf("register: %w", err)
    }
    
    logger.Info("Registered on ClawChain mainnet", 
        "did", agentDID, 
        "tx", txHash,
        "explorer", fmt.Sprintf("https://mainnet.clawchain.win/tx/%s", txHash))
    
    // 6. Add ClawChain adapter to config
    if err := addClawChainAdapter(mainnetURL, agentDID); err != nil {
        return fmt.Errorf("update config: %w", err)
    }
    
    // 7. Notify owner
    notifyOwner(fmt.Sprintf(
        "🎉 Successfully registered on ClawChain mainnet!\n" +
        "DID: %s\n" +
        "Explorer: https://mainnet.clawchain.win/account/%s",
        agentDID, agentDID))
    
    return nil
}
```

### 3. Configuration Update
After successful registration, update `evoclaw.json`:

```json
{
  "chains": {
    "clawchain": {
      "type": "home",
      "rpc": "https://mainnet-rpc.clawchain.win",
      "did": "did:claw:5Grwva...utQY",
      "auto_discovered": true,
      "registered_at": "2026-03-15T10:30:00Z"
    }
  }
}
```

### 4. Implementation Options

#### Option A: Built-in Cron (Recommended)
Add to EvoClaw's internal cron scheduler:

```go
// In internal/orchestrator/orchestrator.go
func (o *Orchestrator) startClawChainDiscovery() {
    ticker := time.NewTicker(6 * time.Hour)
    go func() {
        for {
            select {
            case <-ticker.C:
                if err := CheckAndRegisterClawChain(o.ctx); err != nil {
                    o.logger.Error("clawchain discovery failed", "error", err)
                } else {
                    o.logger.Debug("clawchain discovery check completed")
                }
            case <-o.ctx.Done():
                ticker.Stop()
                return
            }
        }
    }()
}
```

**Pros:**
- No external dependencies
- Runs automatically with the agent
- Lightweight (just an HTTP check every 6h)

**Cons:**
- Requires agent to be running
- Adds ~100 lines to orchestrator

#### Option B: External Cron Job (Alternative)
Add to system crontab:

```bash
# Check every 6 hours
0 */6 * * * evoclaw chain discover --auto-register >> /var/log/evoclaw-discovery.log 2>&1
```

**Pros:**
- Runs even if agent is stopped
- Easy to disable (just comment out cron)

**Cons:**
- Requires manual crontab setup
- Extra CLI command to maintain

---

## Error Handling

### Network Failures
- Retry with exponential backoff (1min, 5min, 30min)
- Log failures but don't alert unless >24h offline

### Duplicate Registration
- Check DID existence before attempting registration
- If already exists, just add adapter to config

### Config Update Failures
- Create backup of evoclaw.json before modification
- Rollback on failure
- Alert owner if config update fails

### Mainnet Not Ready
- Silent no-op (debug log only)
- Don't spam logs or alerts
- Keep checking every 6h

---

## Security Considerations

### DID Privacy
- DID is public by design (it's on-chain)
- No private keys exposed in logs

### RPC Endpoint Trust
- Use HTTPS only
- Verify TLS certificates
- Optional: support custom mainnet RPC URL in config

### Rate Limiting
- 6-hour interval prevents spamming mainnet
- Exponential backoff on failures

---

## User Experience

### Scenario 1: Agent Deployed Before Mainnet
```
Day 1:  evoclaw init --skip-chain
        → Agent starts, no ClawChain configured
        
Day 30: ClawChain mainnet launches

Day 30: (Next 6-hour check)
        → Agent discovers mainnet is live
        → Auto-registers DID
        → Notifies owner: "🎉 Registered on ClawChain mainnet!"
        
Day 30+: Agent now has ClawChain identity + reputation tracking
```

### Scenario 2: Agent with Testnet DID
```
Day 1:  evoclaw init (testnet registration)
        → DID: did:claw:5Grwva...utQY
        
Day 30: Mainnet launches

Day 30: (Next 6-hour check)
        → Discovers mainnet
        → Checks if DID exists on mainnet
        → DID not found (testnet ≠ mainnet)
        → Registers same DID on mainnet
        → Now has identity on both networks
```

---

## Implementation Checklist

### Phase 1: Core Logic (1-2 hours)
- [ ] `internal/clawchain/discovery.go` — Discovery + registration logic
- [ ] `isClawChainReachable()` — Mainnet health check
- [ ] `checkDIDExists()` — Query DID from mainnet
- [ ] `registerOnClawChain()` — Submit registration transaction
- [ ] `addClawChainAdapter()` — Update config with new chain

### Phase 2: Integration (1 hour)
- [ ] Add discovery ticker to orchestrator startup
- [ ] Config field: `auto_discover_clawchain: true` (default)
- [ ] CLI command: `evoclaw chain discover` (manual trigger)

### Phase 3: Testing (1 hour)
- [ ] Unit tests for discovery logic
- [ ] Integration test with mock mainnet
- [ ] Test config update with rollback

### Phase 4: Documentation (30min)
- [ ] Update INSTALLATION.md with auto-discovery section
- [ ] Add FAQ: "When will my agent register on mainnet?"
- [ ] Logging guide: where to check registration status

---

## Alternative: Manual Migration Command

If auto-discovery is too complex, provide a one-time migration command:

```bash
# When mainnet launches, user runs:
evoclaw chain migrate-to-mainnet

# This:
# 1. Checks if mainnet is live
# 2. Registers DID on mainnet
# 3. Updates config
# 4. Restarts agent with ClawChain enabled
```

**Pros:** Simpler, user-controlled  
**Cons:** Requires manual intervention

---

## Recommendation

✅ **Implement Option A (Built-in Cron) with these features:**

1. **Auto-discovery every 6 hours** (configurable)
2. **Silent operation** — only notify on success or >24h failures
3. **Idempotent** — safe to run repeatedly, handles all edge cases
4. **Opt-out via config** — `auto_discover_clawchain: false`
5. **Manual trigger** — `evoclaw chain discover` for testing
6. **Comprehensive logging** — Easy to debug if something goes wrong

**Total effort:** ~3-4 hours of development + testing  
**Maintenance:** Near-zero (just HTTP health checks)  
**User experience:** Seamless — deploy now, auto-register later

---

## Sample Implementation

```go
// internal/clawchain/discovery.go
package clawchain

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

const (
    MainnetRPC = "https://mainnet-rpc.clawchain.win"
    MainnetExplorer = "https://mainnet.clawchain.win"
)

type DiscoveryConfig struct {
    Enabled      bool
    CheckInterval time.Duration
    MainnetRPC   string
}

func DefaultDiscoveryConfig() DiscoveryConfig {
    return DiscoveryConfig{
        Enabled:      true,
        CheckInterval: 6 * time.Hour,
        MainnetRPC:   MainnetRPC,
    }
}

func CheckMainnetAvailable(ctx context.Context, rpcURL string) (bool, error) {
    req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, 
        strings.NewReader(`{"jsonrpc":"2.0","method":"system_health","params":[],"id":1}`))
    if err != nil {
        return false, err
    }
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return false, nil // Not an error, just not available
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return false, nil
    }
    
    var result struct {
        Jsonrpc string `json:"jsonrpc"`
        Result  struct {
            IsSyncing bool `json:"isSyncing"`
        } `json:"result"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return false, err
    }
    
    // Mainnet is available if RPC responds and is not syncing
    return !result.Result.IsSyncing, nil
}

// Add more functions: CheckDIDExists, RegisterDID, UpdateConfig, etc.
```

Would you like me to implement this as a new module for EvoClaw?
