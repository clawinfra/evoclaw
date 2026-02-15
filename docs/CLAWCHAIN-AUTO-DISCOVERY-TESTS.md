# ClawChain Auto-Discovery Test Plan

## Test Coverage Goal: 90%+

All new code in `internal/clawchain/discovery.go` must be thoroughly tested to maintain EvoClaw's 84.2% overall coverage.

---

## Unit Tests

### 1. Mainnet Availability Check
**File:** `internal/clawchain/discovery_test.go`

```go
func TestCheckMainnetAvailable_Success(t *testing.T) {
    // Mock HTTP server returning healthy response
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "result": map[string]interface{}{
                "isSyncing": false,
                "peers":     10,
            },
            "id": 1,
        })
    }))
    defer server.Close()

    available, err := CheckMainnetAvailable(context.Background(), server.URL)
    assert.NoError(t, err)
    assert.True(t, available)
}

func TestCheckMainnetAvailable_Syncing(t *testing.T) {
    // Mock server returning syncing=true
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "result": map[string]interface{}{
                "isSyncing": true, // Still syncing
            },
            "id": 1,
        })
    }))
    defer server.Close()

    available, err := CheckMainnetAvailable(context.Background(), server.URL)
    assert.NoError(t, err)
    assert.False(t, available) // Not available yet
}

func TestCheckMainnetAvailable_Unreachable(t *testing.T) {
    // No server running
    available, err := CheckMainnetAvailable(context.Background(), "http://localhost:99999")
    assert.NoError(t, err) // Not an error, just not available
    assert.False(t, available)
}

func TestCheckMainnetAvailable_InvalidResponse(t *testing.T) {
    // Server returns invalid JSON
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("invalid json"))
    }))
    defer server.Close()

    available, err := CheckMainnetAvailable(context.Background(), server.URL)
    assert.Error(t, err)
    assert.False(t, available)
}

func TestCheckMainnetAvailable_Timeout(t *testing.T) {
    // Server that never responds
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(30 * time.Second)
    }))
    defer server.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    available, err := CheckMainnetAvailable(ctx, server.URL)
    assert.NoError(t, err) // Timeout is treated as "not available"
    assert.False(t, available)
}
```

### 2. DID Existence Check
```go
func TestCheckDIDExists_Found(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Mock RPC call: state_getStorage for DID
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "result":  "0x01234567", // Non-null means DID exists
            "id":      1,
        })
    }))
    defer server.Close()

    exists, err := CheckDIDExists(context.Background(), server.URL, "did:claw:5Grwva...utQY")
    assert.NoError(t, err)
    assert.True(t, exists)
}

func TestCheckDIDExists_NotFound(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "result":  nil, // Null means DID doesn't exist
            "id":      1,
        })
    }))
    defer server.Close()

    exists, err := CheckDIDExists(context.Background(), server.URL, "did:claw:5Grwva...utQY")
    assert.NoError(t, err)
    assert.False(t, exists)
}
```

### 3. Registration Flow
```go
func TestRegisterOnClawChain_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Mock successful registration
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "result":  "0xabcdef1234567890", // Transaction hash
            "id":      1,
        })
    }))
    defer server.Close()

    txHash, err := RegisterOnClawChain(context.Background(), server.URL, "did:claw:5Grwva...utQY")
    assert.NoError(t, err)
    assert.NotEmpty(t, txHash)
}

func TestRegisterOnClawChain_AlreadyRegistered(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Mock error: DID already exists
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "jsonrpc": "2.0",
            "error": map[string]interface{}{
                "code":    -32000,
                "message": "DID already registered",
            },
            "id": 1,
        })
    }))
    defer server.Close()

    _, err := RegisterOnClawChain(context.Background(), server.URL, "did:claw:5Grwva...utQY")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "already registered")
}
```

### 4. Config Update
```go
func TestAddClawChainAdapter_Success(t *testing.T) {
    // Create temp config file
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "evoclaw.json")
    
    initialConfig := map[string]interface{}{
        "channels": map[string]interface{}{"tui": map[string]bool{"enabled": true}},
        "chains":   map[string]interface{}{},
    }
    data, _ := json.MarshalIndent(initialConfig, "", "  ")
    os.WriteFile(configPath, data, 0644)

    // Add ClawChain adapter
    err := AddClawChainAdapter(configPath, "https://mainnet-rpc.clawchain.win", "did:claw:5Grwva...utQY")
    assert.NoError(t, err)

    // Verify config was updated
    var updated map[string]interface{}
    data, _ = os.ReadFile(configPath)
    json.Unmarshal(data, &updated)
    
    chains := updated["chains"].(map[string]interface{})
    clawchain := chains["clawchain"].(map[string]interface{})
    
    assert.Equal(t, "home", clawchain["type"])
    assert.Equal(t, "https://mainnet-rpc.clawchain.win", clawchain["rpc"])
    assert.Equal(t, "did:claw:5Grwva...utQY", clawchain["did"])
    assert.Equal(t, true, clawchain["auto_discovered"])
}

func TestAddClawChainAdapter_BackupOnFailure(t *testing.T) {
    // Create config that will cause update failure
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "evoclaw.json")
    os.WriteFile(configPath, []byte("invalid json"), 0644)

    // Attempt to add adapter
    err := AddClawChainAdapter(configPath, "https://mainnet-rpc.clawchain.win", "did:claw:test")
    assert.Error(t, err)

    // Verify backup was created
    backupPath := configPath + ".backup"
    assert.FileExists(t, backupPath)
    
    backupData, _ := os.ReadFile(backupPath)
    assert.Equal(t, "invalid json", string(backupData))
}
```

### 5. End-to-End Discovery Flow
```go
func TestCheckAndRegisterClawChain_FullFlow(t *testing.T) {
    // Mock mainnet RPC
    mainnetCalls := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mainnetCalls++
        
        var req map[string]interface{}
        json.NewDecoder(r.Body).Decode(&req)
        method := req["method"].(string)
        
        w.Header().Set("Content-Type", "application/json")
        
        switch method {
        case "system_health":
            // Mainnet is available
            json.NewEncoder(w).Encode(map[string]interface{}{
                "jsonrpc": "2.0",
                "result":  map[string]interface{}{"isSyncing": false},
                "id":      req["id"],
            })
        case "state_getStorage":
            // DID doesn't exist yet
            json.NewEncoder(w).Encode(map[string]interface{}{
                "jsonrpc": "2.0",
                "result":  nil,
                "id":      req["id"],
            })
        case "author_submitExtrinsic":
            // Registration successful
            json.NewEncoder(w).Encode(map[string]interface{}{
                "jsonrpc": "2.0",
                "result":  "0xabcdef1234567890",
                "id":      req["id"],
            })
        }
    }))
    defer server.Close()

    // Create temp config
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "evoclaw.json")
    os.WriteFile(configPath, []byte(`{"chains":{}}`), 0644)

    // Run discovery
    cfg := DiscoveryConfig{
        Enabled:      true,
        MainnetRPC:   server.URL,
        ConfigPath:   configPath,
    }
    
    err := CheckAndRegisterClawChain(context.Background(), cfg)
    assert.NoError(t, err)
    
    // Verify mainnet was queried
    assert.Greater(t, mainnetCalls, 0)
    
    // Verify config was updated
    var updated map[string]interface{}
    data, _ := os.ReadFile(configPath)
    json.Unmarshal(data, &updated)
    assert.Contains(t, updated["chains"], "clawchain")
}

func TestCheckAndRegisterClawChain_AlreadyConfigured(t *testing.T) {
    // Config already has ClawChain
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "evoclaw.json")
    os.WriteFile(configPath, []byte(`{"chains":{"clawchain":{"rpc":"https://test"}}}`), 0644)

    cfg := DiscoveryConfig{
        Enabled:    true,
        ConfigPath: configPath,
    }
    
    err := CheckAndRegisterClawChain(context.Background(), cfg)
    assert.NoError(t, err)
    
    // Should skip without making any RPC calls
}

func TestCheckAndRegisterClawChain_MainnetNotReady(t *testing.T) {
    // No server running = mainnet not ready
    cfg := DiscoveryConfig{
        Enabled:    true,
        MainnetRPC: "http://localhost:99999",
        ConfigPath: "/tmp/test-config.json",
    }
    
    err := CheckAndRegisterClawChain(context.Background(), cfg)
    assert.NoError(t, err) // Not an error, just not ready
}
```

---

## Integration Tests

### File: `integration/clawchain_discovery_test.go`

```go
//go:build integration
// +build integration

package integration

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
)

func TestDiscoveryCron_Lifecycle(t *testing.T) {
    // Start orchestrator with discovery enabled
    cfg := testConfig()
    cfg.Discovery.Enabled = true
    cfg.Discovery.CheckInterval = 2 * time.Second // Short interval for testing
    
    orch := orchestrator.New(cfg, testLogger())
    
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    go orch.Start(ctx)
    
    // Wait for at least 2 discovery checks
    time.Sleep(5 * time.Second)
    
    // Verify discovery ran
    stats := orch.GetDiscoveryStats()
    assert.GreaterOrEqual(t, stats.CheckCount, 2)
}
```

---

## CI Pipeline Updates

### `.github/workflows/ci.yml` Changes

No changes needed! The existing `go test -race -coverprofile=coverage.out` will automatically:
1. Run all new `*_test.go` files
2. Include coverage from `internal/clawchain/discovery_test.go`
3. Verify coverage stays above 55% threshold

Current coverage: **84.2%**  
New code target: **90%+**  
Overall coverage after: **~84-85%** (should stay above 84%)

---

## Manual Test Checklist

Before merging:

- [ ] Run `go test ./internal/clawchain/... -v -cover`
- [ ] Verify coverage: `go test ./internal/clawchain/... -coverprofile=coverage.out && go tool cover -func=coverage.out`
- [ ] Run full test suite: `go test -race ./...`
- [ ] Check lint: `golangci-lint run --timeout=5m`
- [ ] Build: `go build -o evoclaw ./cmd/evoclaw`
- [ ] Integration test (manual): Start orchestrator, verify discovery logs appear every 6h
- [ ] Config validation: Verify `evoclaw.json` is updated correctly after mock registration

---

## Coverage Targets by Function

| Function | Target Coverage | Test Cases |
|----------|----------------|------------|
| `CheckMainnetAvailable` | 100% | Success, syncing, unreachable, timeout, invalid response |
| `CheckDIDExists` | 100% | Found, not found, RPC error, network failure |
| `RegisterOnClawChain` | 100% | Success, already registered, network error, invalid DID |
| `AddClawChainAdapter` | 100% | Success, backup on failure, read-only filesystem |
| `CheckAndRegisterClawChain` | 95% | Full flow, already configured, mainnet not ready, DID exists |
| `startDiscoveryCron` | 80% | Lifecycle test (integration only) |

**Overall target:** 90-95% coverage for all new discovery code

---

## Test Execution

```bash
# Unit tests only
go test ./internal/clawchain/... -v -cover

# With coverage report
go test ./internal/clawchain/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Integration tests
go test -tags=integration ./integration/... -v

# Full CI simulation
go test -race -coverprofile=coverage.out ./...
golangci-lint run --timeout=5m
go build -o evoclaw ./cmd/evoclaw
```

---

## Expected Coverage Report

After implementing all tests:

```
internal/clawchain/discovery.go:
    CheckMainnetAvailable           100.0%
    CheckDIDExists                  100.0%
    RegisterOnClawChain             100.0%
    AddClawChainAdapter             100.0%
    CheckAndRegisterClawChain        95.0%
    startDiscoveryCron               80.0% (integration only)
    ────────────────────────────────────
    TOTAL                            94.2%
```

This maintains EvoClaw's 84.2% overall coverage and keeps CI green ✅
