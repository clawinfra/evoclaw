// Package clawchain provides ClawChain Substrate node integration.
// This file implements the DID auto-discovery module (ADR-003): on startup,
// EvoClaw checks whether its agent DID is registered on ClawChain and, if not,
// submits a register_did extrinsic via a Python subprocess.
package clawchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DiscoveryConfig holds all settings for the ClawChain DID auto-discovery loop.
type DiscoveryConfig struct {
	// Enabled turns the discovery loop on or off.
	Enabled bool `json:"enabled"`
	// NodeURL is the ClawChain Substrate HTTP RPC endpoint.
	// Default: "http://testnet.clawchain.win:9944"
	NodeURL string `json:"node_url"`
	// CheckInterval is how frequently to poll for DID registration.
	// Default: 6 hours.
	CheckInterval time.Duration `json:"check_interval"`
	// AgentSeed is the sr25519 seed phrase for signing extrinsics.
	// Use well-known dev paths such as "//Alice" for testnet.
	// Store secrets securely (e.g. environment variable) for production.
	AgentSeed string `json:"agent_seed"`
	// AgentContext is the W3C DID context URL embedded in the DID document.
	AgentContext string `json:"agent_context"`
	// RPCEndpoint is EvoClaw's own WebSocket RPC URL, registered in the DID document.
	RPCEndpoint string `json:"rpc_endpoint"`
	// AccountIDHex is the 32-byte raw public key (hex, optional "0x" prefix).
	// If empty, the discoverer attempts to derive it from AgentSeed for
	// well-known dev accounts (//Alice, //Bob, //Charlie, //Dave, //Eve, //Ferdie).
	AccountIDHex string `json:"account_id_hex,omitempty"`
}

// DefaultDiscoveryConfig returns a DiscoveryConfig with production-safe defaults.
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Enabled:       true,
		NodeURL:       "http://testnet.clawchain.win:9944",
		CheckInterval: 6 * time.Hour,
		AgentContext:  "https://www.w3.org/ns/did/v1",
	}
}

// DiscoveryResult describes the outcome of a single RunOnce cycle.
type DiscoveryResult struct {
	// AlreadyRegistered is true when the DID was found on-chain at the start
	// of the cycle (no registration was attempted).
	AlreadyRegistered bool `json:"already_registered"`
	// Registered is true when the DID was successfully registered during this cycle.
	Registered bool `json:"registered"`
	// TxHash is the block hash returned by the register_did extrinsic, if submitted.
	TxHash string `json:"tx_hash,omitempty"`
	// Error is a human-readable error message when the cycle fails.
	Error string `json:"error,omitempty"`
}

// Discoverer performs periodic ClawChain DID auto-discovery.
// Construct with NewDiscoverer (production) or NewDiscovererWithCaller (testing).
type Discoverer struct {
	cfg    DiscoveryConfig
	logger *slog.Logger
	caller RPCCaller
}

// NewDiscoverer creates a Discoverer using the default HTTP JSON-RPC caller.
// The caller timeout is set to 15 seconds.
func NewDiscoverer(cfg DiscoveryConfig, logger *slog.Logger) *Discoverer {
	return &Discoverer{
		cfg:    cfg,
		logger: logger,
		caller: &httpCaller{timeout: 15 * time.Second},
	}
}

// NewDiscovererWithCaller creates a Discoverer with a custom RPCCaller.
// Use this in tests to inject a mockRPCCaller.
func NewDiscovererWithCaller(cfg DiscoveryConfig, logger *slog.Logger, caller RPCCaller) *Discoverer {
	return &Discoverer{
		cfg:    cfg,
		logger: logger,
		caller: caller,
	}
}

// CheckReachable pings the node via system_health and returns (true, nil) when
// the node is up and not syncing. Returns (false, err) on transport errors, and
// (false, nil) when the node is still syncing.
func (d *Discoverer) CheckReachable(ctx context.Context) (bool, error) {
	req := SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "system_health",
		Params:  []interface{}{},
	}

	resp, err := d.caller.Call(ctx, d.cfg.NodeURL, req)
	if err != nil {
		return false, fmt.Errorf("system_health RPC failed: %w", err)
	}
	if resp.Error != nil {
		return false, fmt.Errorf("system_health RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	// resp.Result is interface{}; marshal→unmarshal to extract isSyncing.
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		return false, fmt.Errorf("marshal system_health result: %w", err)
	}

	var health struct {
		IsSyncing bool `json:"isSyncing"`
	}
	if err := json.Unmarshal(raw, &health); err != nil {
		return false, fmt.Errorf("unmarshal system_health: %w", err)
	}

	if health.IsSyncing {
		d.logger.Info("clawchain node is syncing; will retry later")
		return false, nil
	}

	return true, nil
}

// CheckDIDRegistered queries state_getStorage for AgentDid.DIDDocuments[accountIDBytes].
// Returns true when a DID document exists on-chain for the given 32-byte account ID.
func (d *Discoverer) CheckDIDRegistered(ctx context.Context, accountIDBytes []byte) (bool, error) {
	storageKey := ComputeStorageKey("AgentDid", "DIDDocuments", accountIDBytes)

	req := SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "state_getStorage",
		Params:  []interface{}{storageKey},
	}

	resp, err := d.caller.Call(ctx, d.cfg.NodeURL, req)
	if err != nil {
		return false, fmt.Errorf("state_getStorage RPC failed: %w", err)
	}
	if resp.Error != nil {
		return false, fmt.Errorf("state_getStorage RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	// A null result means the key does not exist (DID not registered).
	// A non-null hex string means the DID document is present.
	if resp.Result == nil {
		return false, nil
	}
	// The result may be a JSON null marshalled to interface{}.
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		return false, fmt.Errorf("marshal state_getStorage result: %w", err)
	}
	if string(raw) == "null" {
		return false, nil
	}

	return true, nil
}

// registerDIDScript is the minimal Python script executed by RegisterDID.
// It uses substrate-interface to submit the register_did extrinsic and prints
// the block hash on success, or writes "ERROR:<message>" to stderr and exits 1.
const registerDIDScript = `
import sys
from substrateinterface import SubstrateInterface, Keypair

seed = sys.argv[1]
node_url = sys.argv[2]
context_str = sys.argv[3]

substrate = SubstrateInterface(url=node_url)
kp = Keypair.create_from_uri(seed)

call = substrate.compose_call(
    call_module='AgentDid',
    call_function='register_did',
    call_params={'context': context_str.encode().hex()}
)
ext = substrate.create_signed_extrinsic(call=call, keypair=kp)
receipt = substrate.submit_extrinsic(ext, wait_for_inclusion=True)
if not receipt.is_success:
    print('ERROR:' + str(receipt.error_message), file=sys.stderr)
    sys.exit(1)
print(receipt.block_hash)
`

// RegisterDID submits the register_did extrinsic by invoking a Python
// subprocess that uses substrate-interface. Returns the block hash on success.
//
// Requirements: python3 with substrateinterface installed
// (pip install substrate-interface).
func (d *Discoverer) RegisterDID(ctx context.Context) (string, error) {
	if d.cfg.AgentSeed == "" {
		return "", fmt.Errorf("AgentSeed must be set to register a DID")
	}
	if d.cfg.AgentContext == "" {
		return "", fmt.Errorf("AgentContext must be set to register a DID")
	}

	// Write the embedded Python script to a temp file.
	tmpFile, err := os.CreateTemp("", "register_did_*.py")
	if err != nil {
		return "", fmt.Errorf("create temp script: %w", err)
	}
	defer os.Remove(tmpFile.Name()) //nolint:errcheck

	if _, err := tmpFile.WriteString(registerDIDScript); err != nil {
		tmpFile.Close() //nolint:errcheck
		return "", fmt.Errorf("write temp script: %w", err)
	}
	tmpFile.Close() //nolint:errcheck

	nodeURL := d.cfg.NodeURL
	if nodeURL == "" {
		nodeURL = "http://testnet.clawchain.win:9944"
	}

	cmd := exec.CommandContext(ctx,
		"python3", tmpFile.Name(),
		d.cfg.AgentSeed,
		nodeURL,
		d.cfg.AgentContext,
	)

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("register_did extrinsic failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("register_did subprocess: %w", err)
	}

	txHash := strings.TrimSpace(string(out))
	if txHash == "" {
		return "", fmt.Errorf("register_did returned empty block hash")
	}

	return txHash, nil
}

// accountBytes resolves the 32-byte account public key for the configured agent.
//
// Priority:
//  1. cfg.AccountIDHex — use directly (supports "0x" prefix).
//  2. Well-known dev seeds (//Alice … //Ferdie) — hardcoded public keys.
//  3. Error — caller must set AccountIDHex or use a well-known seed.
func (d *Discoverer) accountBytes() ([]byte, error) {
	if d.cfg.AccountIDHex != "" {
		raw := strings.TrimPrefix(d.cfg.AccountIDHex, "0x")
		b, err := hex.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid account_id_hex: %w", err)
		}
		if len(b) != 32 {
			return nil, fmt.Errorf("account_id_hex must decode to exactly 32 bytes (got %d)", len(b))
		}
		return b, nil
	}

	// Well-known Substrate dev account seeds → raw SR25519 public keys.
	wellKnown := map[string]string{
		"//Alice":  "d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d",
		"//Bob":    "8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48",
		"//Charlie": "90b5ab205c6974c9ea841be688864633dc9ca8a357843eeacf2314649965fe22",
		"//Dave":   "306721211d5404bd9da88e0204360a1a9ab8b87c66c1bc2fcdd37f3c2222cc20",
		"//Eve":    "e659a7a1628cdd93febc04a4e0646ea20e9f5f0ce097d9a05290d4a9e054df4e",
		"//Ferdie": "1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c",
	}

	if pubHex, ok := wellKnown[d.cfg.AgentSeed]; ok {
		b, _ := hex.DecodeString(pubHex)
		return b, nil
	}

	return nil, fmt.Errorf(
		"cannot derive account bytes from seed %q; set account_id_hex in DiscoveryConfig or use a well-known dev seed (//Alice, //Bob, …)",
		d.cfg.AgentSeed,
	)
}

// RunOnce performs one full DID discovery cycle:
//  1. Verify the node is reachable and not syncing.
//  2. Look up the agent's DID document on-chain.
//  3. If absent, submit the register_did extrinsic.
//
// The result is never nil; errors are embedded in DiscoveryResult.Error.
func (d *Discoverer) RunOnce(ctx context.Context) (*DiscoveryResult, error) {
	result := &DiscoveryResult{}

	// Step 1: liveness check.
	reachable, err := d.CheckReachable(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("node unreachable: %v", err)
		d.logger.Warn("clawchain discovery: node unreachable", "error", err)
		return result, nil //nolint:nilerr — error is embedded in result
	}
	if !reachable {
		result.Error = "node is syncing; skipping DID check"
		d.logger.Info("clawchain discovery: node syncing, skipping cycle")
		return result, nil
	}

	// Step 2: resolve account bytes.
	acctBytes, err := d.accountBytes()
	if err != nil {
		result.Error = fmt.Sprintf("resolve account: %v", err)
		d.logger.Error("clawchain discovery: cannot resolve account bytes", "error", err)
		return result, nil //nolint:nilerr
	}

	// Step 3: check on-chain DID registration.
	registered, err := d.CheckDIDRegistered(ctx, acctBytes)
	if err != nil {
		result.Error = fmt.Sprintf("check DID: %v", err)
		d.logger.Error("clawchain discovery: state query failed", "error", err)
		return result, nil //nolint:nilerr
	}

	if registered {
		result.AlreadyRegistered = true
		d.logger.Info("clawchain discovery: DID already registered")
		return result, nil
	}

	// Step 4: register DID.
	d.logger.Info("clawchain discovery: DID not found on-chain; submitting register_did extrinsic")
	txHash, err := d.RegisterDID(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("register DID: %v", err)
		d.logger.Error("clawchain discovery: registration failed", "error", err)
		return result, nil //nolint:nilerr
	}

	result.Registered = true
	result.TxHash = txHash
	d.logger.Info("clawchain discovery: DID registered successfully", "tx_hash", txHash)
	return result, nil
}

// Start runs RunOnce immediately and then repeats every cfg.CheckInterval.
// It blocks until ctx is cancelled. Designed to be launched in a goroutine.
func (d *Discoverer) Start(ctx context.Context) {
	d.logger.Info("clawchain auto-discovery starting",
		"node_url", d.cfg.NodeURL,
		"interval", d.cfg.CheckInterval,
	)

	// Run immediately on startup.
	if result, err := d.RunOnce(ctx); err != nil {
		d.logger.Error("clawchain discovery error", "error", err)
	} else if result.Error != "" {
		d.logger.Warn("clawchain discovery cycle failed", "error", result.Error)
	}

	interval := d.cfg.CheckInterval
	if interval <= 0 {
		interval = 6 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("clawchain auto-discovery stopped")
			return
		case <-ticker.C:
			if result, err := d.RunOnce(ctx); err != nil {
				d.logger.Error("clawchain discovery error", "error", err)
			} else if result.Error != "" {
				d.logger.Warn("clawchain discovery cycle failed", "error", result.Error)
			}
		}
	}
}
