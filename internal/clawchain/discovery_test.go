package clawchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// mockRPCCaller implements RPCCaller for unit tests.
// It maps JSON-RPC method names to canned responses.
type mockRPCCaller struct {
	// responses maps method name → pre-marshalled result (interface{}).
	responses map[string]interface{}
	// err, if non-nil, is returned for every Call instead of a response.
	err error
}

func (m *mockRPCCaller) Call(_ context.Context, _ string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	result, ok := m.responses[req.Method]
	if !ok {
		return &SubstrateRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  nil,
		}, nil
	}
	return &SubstrateRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}, nil
}

// healthResult builds a system_health result map.
func healthResult(isSyncing bool) interface{} {
	return map[string]interface{}{
		"isSyncing":       isSyncing,
		"peers":           5,
		"shouldHavePeers": true,
	}
}

// nullLogger returns a slog logger that discards all output.
func nullLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 4}))
}

// aliceBytes returns Alice's well-known 32-byte public key.
func aliceBytes() []byte {
	b, _ := hex.DecodeString("d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	return b
}

// testCfg returns a minimal DiscoveryConfig usable in tests.
func testCfg() DiscoveryConfig {
	return DiscoveryConfig{
		Enabled:       true,
		NodeURL:       "http://localhost:9944",
		CheckInterval: time.Minute,
		AgentSeed:     "//Alice",
		AgentContext:  "https://www.w3.org/ns/did/v1",
	}
}

// ── DefaultDiscoveryConfig ────────────────────────────────────────────────────

func TestDefaultDiscoveryConfig(t *testing.T) {
	cfg := DefaultDiscoveryConfig()

	if !cfg.Enabled {
		t.Error("default config should have Enabled=true")
	}
	if cfg.NodeURL != "http://testnet.clawchain.win:9944" {
		t.Errorf("unexpected NodeURL: %q", cfg.NodeURL)
	}
	if cfg.CheckInterval != 6*time.Hour {
		t.Errorf("unexpected CheckInterval: %v", cfg.CheckInterval)
	}
	if cfg.AgentContext != "https://www.w3.org/ns/did/v1" {
		t.Errorf("unexpected AgentContext: %q", cfg.AgentContext)
	}
}

// ── NewDiscoverer / NewDiscovererWithCaller ───────────────────────────────────

func TestNewDiscoverer(t *testing.T) {
	cfg := DefaultDiscoveryConfig()
	d := NewDiscoverer(cfg, nullLogger())
	if d == nil {
		t.Fatal("NewDiscoverer returned nil")
	}
	if d.caller == nil {
		t.Error("caller should be set to default httpCaller")
	}
}

func TestNewDiscovererWithCaller(t *testing.T) {
	mock := &mockRPCCaller{}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)
	if d == nil {
		t.Fatal("NewDiscovererWithCaller returned nil")
	}
	if d.caller != mock {
		t.Error("caller should be the injected mock")
	}
}

// ── CheckReachable ────────────────────────────────────────────────────────────

func TestCheckReachable_Healthy(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health": healthResult(false),
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckReachable(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected reachable=true for healthy, non-syncing node")
	}
}

func TestCheckReachable_NodeDown(t *testing.T) {
	mock := &mockRPCCaller{err: errors.New("connection refused")}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckReachable(context.Background())
	if err == nil {
		t.Error("expected error when node is down")
	}
	if ok {
		t.Error("expected reachable=false when node is down")
	}
}

func TestCheckReachable_Syncing(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health": healthResult(true),
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckReachable(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected reachable=false while node is syncing")
	}
}

func TestCheckReachable_RPCError(t *testing.T) {
	// Simulate an RPC-level error response.
	mock := &mockRPCCaller{} // no entry for system_health → returns nil result (treated as unknown)
	// Override Call to inject an RPC error.
	errCaller := &rpcErrorCaller{code: -32600, msg: "Invalid request"}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), errCaller)

	ok, err := d.CheckReachable(context.Background())
	if err == nil {
		t.Error("expected error on RPC-level error response")
	}
	if ok {
		t.Error("expected reachable=false on RPC error")
	}
	_ = mock
}

// rpcErrorCaller returns an RPC-level error (not a transport error).
type rpcErrorCaller struct{ code int; msg string }

func (r *rpcErrorCaller) Call(_ context.Context, _ string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error) {
	return &SubstrateRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &SubstrateRPCErr{Code: r.code, Message: r.msg},
	}, nil
}

// ── CheckDIDRegistered ────────────────────────────────────────────────────────

func TestCheckDIDRegistered_Exists(t *testing.T) {
	// state_getStorage returns a non-null hex string → DID exists.
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"state_getStorage": "0xdeadbeef01020304",
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckDIDRegistered(context.Background(), aliceBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected registered=true when storage returns a value")
	}
}

func TestCheckDIDRegistered_NotExists(t *testing.T) {
	// state_getStorage returns null → DID not registered.
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"state_getStorage": nil,
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckDIDRegistered(context.Background(), aliceBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected registered=false when storage returns null")
	}
}

func TestCheckDIDRegistered_TransportError(t *testing.T) {
	mock := &mockRPCCaller{err: errors.New("dial tcp: connection refused")}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	ok, err := d.CheckDIDRegistered(context.Background(), aliceBytes())
	if err == nil {
		t.Error("expected error on transport failure")
	}
	if ok {
		t.Error("expected registered=false on transport error")
	}
}

func TestCheckDIDRegistered_RPCError(t *testing.T) {
	errCaller := &rpcErrorCaller{code: -32000, msg: "storage key not found"}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), errCaller)

	ok, err := d.CheckDIDRegistered(context.Background(), aliceBytes())
	if err == nil {
		t.Error("expected error on RPC-level error response")
	}
	if ok {
		t.Error("expected registered=false on RPC error")
	}
}

// ── accountBytes ──────────────────────────────────────────────────────────────

func TestAccountBytes_WellKnownAlice(t *testing.T) {
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), &mockRPCCaller{})
	b, err := d.accountBytes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(b))
	}
	wantHex := "d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"
	if hex.EncodeToString(b) != wantHex {
		t.Errorf("Alice public key mismatch: got %x, want %s", b, wantHex)
	}
}

func TestAccountBytes_ExplicitHex(t *testing.T) {
	cfg := testCfg()
	cfg.AccountIDHex = "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
	b, err := d.accountBytes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(b))
	}
}

func TestAccountBytes_InvalidHex(t *testing.T) {
	cfg := testCfg()
	cfg.AccountIDHex = "0xNOTHEX"
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
	_, err := d.accountBytes()
	if err == nil {
		t.Error("expected error for invalid hex account ID")
	}
}

func TestAccountBytes_WrongLength(t *testing.T) {
	cfg := testCfg()
	cfg.AccountIDHex = "0xdeadbeef" // only 4 bytes
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
	_, err := d.accountBytes()
	if err == nil {
		t.Error("expected error for account ID shorter than 32 bytes")
	}
}

func TestAccountBytes_UnknownSeed(t *testing.T) {
	cfg := testCfg()
	cfg.AgentSeed = "//Unknown"
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
	_, err := d.accountBytes()
	if err == nil {
		t.Error("expected error for unknown seed without explicit AccountIDHex")
	}
}

// TestAccountBytes_AllWellKnown verifies all dev accounts are resolvable.
func TestAccountBytes_AllWellKnown(t *testing.T) {
	seeds := []string{"//Alice", "//Bob", "//Charlie", "//Dave", "//Eve", "//Ferdie"}
	for _, seed := range seeds {
		cfg := testCfg()
		cfg.AgentSeed = seed
		d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
		b, err := d.accountBytes()
		if err != nil {
			t.Errorf("accountBytes(%q) unexpected error: %v", seed, err)
			continue
		}
		if len(b) != 32 {
			t.Errorf("accountBytes(%q) length = %d, want 32", seed, len(b))
		}
	}
}

// ── RunOnce ───────────────────────────────────────────────────────────────────

func TestRunOnce_AlreadyRegistered(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": "0xdeadbeef",
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result must not be nil")
	}
	if !result.AlreadyRegistered {
		t.Error("expected AlreadyRegistered=true when DID found on-chain")
	}
	if result.Registered {
		t.Error("expected Registered=false (no new registration needed)")
	}
	if result.Error != "" {
		t.Errorf("unexpected error in result: %s", result.Error)
	}
}

func TestRunOnce_NodeUnreachable(t *testing.T) {
	mock := &mockRPCCaller{err: errors.New("connection refused")}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected Go error (should be embedded in result): %v", err)
	}
	if result == nil {
		t.Fatal("result must not be nil")
	}
	if result.Error == "" {
		t.Error("expected error message in result when node is unreachable")
	}
	if result.AlreadyRegistered || result.Registered {
		t.Error("should not be registered when node is unreachable")
	}
}

func TestRunOnce_NodeSyncing(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health": healthResult(true),
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error message when node is syncing")
	}
	if result.AlreadyRegistered || result.Registered {
		t.Error("should not attempt DID check while syncing")
	}
}

func TestRunOnce_CheckDIDFails(t *testing.T) {
	// Health check passes but state_getStorage fails.
	healthMock := &twoMethodCaller{
		healthResp:  healthResult(false),
		storageErr:  errors.New("storage layer error"),
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), healthMock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error message when state_getStorage fails")
	}
}

func TestRunOnce_BadSeed(t *testing.T) {
	cfg := testCfg()
	cfg.AgentSeed = "//UnknownSeed"
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health": healthResult(false),
		},
	}
	d := NewDiscovererWithCaller(cfg, nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error when account bytes cannot be resolved")
	}
}

// twoMethodCaller lets us return different responses/errors for health vs storage.
type twoMethodCaller struct {
	healthResp  interface{}
	storageErr  error
	storageResp interface{}
}

func (t *twoMethodCaller) Call(_ context.Context, _ string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error) {
	switch req.Method {
	case "system_health":
		return &SubstrateRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: t.healthResp}, nil
	case "state_getStorage":
		if t.storageErr != nil {
			return nil, t.storageErr
		}
		return &SubstrateRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: t.storageResp}, nil
	default:
		return &SubstrateRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: nil}, nil
	}
}

// ── RegisterDID ───────────────────────────────────────────────────────────────

func TestRegisterDID_Skipped(t *testing.T) {
	// This test exercises only the missing-seed validation path (no subprocess).
	cfg := testCfg()
	cfg.AgentSeed = ""
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})

	_, err := d.RegisterDID(context.Background())
	if err == nil {
		t.Error("expected error when AgentSeed is empty")
	}
}

func TestRegisterDID_MissingContext(t *testing.T) {
	cfg := testCfg()
	cfg.AgentContext = ""
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})

	_, err := d.RegisterDID(context.Background())
	if err == nil {
		t.Error("expected error when AgentContext is empty")
	}
}

// TestRegisterDID_PythonSubprocess is skipped in CI because it requires
// python3 with substrate-interface installed.
func TestRegisterDID_PythonSubprocess(t *testing.T) {
	t.Skip("requires python3 + substrate-interface; run manually against testnet")
}

// writeFakePython3 writes a shell script named "python3" to dir that prints
// output and exits with code. Returns the dir for PATH prepending.
func writeFakePython3(t *testing.T, dir, output string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\n"
	if output != "" {
		script += "echo " + output + "\n"
	}
	if exitCode != 0 {
		script += "echo 'ERROR: fake failure' >&2\n"
		script += "exit 1\n"
	}
	p := filepath.Join(dir, "python3")
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatalf("write fake python3: %v", err)
	}
}

// TestRegisterDID_Success uses a fake python3 shell script to exercise the
// success path without requiring the real substrate-interface library.
func TestRegisterDID_Success(t *testing.T) {
	dir := t.TempDir()
	writeFakePython3(t, dir, "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab", 0)

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+":"+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	defer func() {
		if err := os.Setenv("PATH", origPath); err != nil {
			t.Logf("failed to restore PATH: %v", err)
		}
	}()

	d := NewDiscovererWithCaller(testCfg(), nullLogger(), &mockRPCCaller{})
	txHash, err := d.RegisterDID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if txHash == "" {
		t.Error("expected non-empty tx hash on success")
	}
}

// TestRegisterDID_ExitError covers the ExitError stderr path.
func TestRegisterDID_ExitError(t *testing.T) {
	dir := t.TempDir()
	writeFakePython3(t, dir, "", 1)

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+":"+origPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	defer func() {
		if err := os.Setenv("PATH", origPath); err != nil {
			t.Logf("failed to restore PATH: %v", err)
		}
	}()

	d := NewDiscovererWithCaller(testCfg(), nullLogger(), &mockRPCCaller{})
	_, err := d.RegisterDID(context.Background())
	if err == nil {
		t.Error("expected error when python3 exits non-zero")
	}
}

// TestRegisterDID_NotFound covers the case where python3 is not in PATH.
func TestRegisterDID_NotFound(t *testing.T) {
	// Use an empty PATH so python3 cannot be found.
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", t.TempDir()) // temp dir with no python3
	defer os.Setenv("PATH", origPath)

	d := NewDiscovererWithCaller(testCfg(), nullLogger(), &mockRPCCaller{})
	_, err := d.RegisterDID(context.Background())
	if err == nil {
		t.Error("expected error when python3 is not in PATH")
	}
}

// TestRegisterDID_DefaultNodeURL verifies that the default node URL is applied
// when cfg.NodeURL is empty.
func TestRegisterDID_DefaultNodeURL(t *testing.T) {
	dir := t.TempDir()
	writeFakePython3(t, dir, "0xdeadbeef", 0)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	cfg := testCfg()
	cfg.NodeURL = "" // empty — should default to testnet URL
	d := NewDiscovererWithCaller(cfg, nullLogger(), &mockRPCCaller{})
	_, err := d.RegisterDID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── RunOnce — registration paths ─────────────────────────────────────────────

// TestRunOnce_RegistrationSuccess covers the full happy path: node healthy,
// DID absent, registration succeeds via fake python3.
func TestRunOnce_RegistrationSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFakePython3(t, dir, "0xdeadbeef1234", 0)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": nil, // not registered
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Registered {
		t.Errorf("expected Registered=true, got error=%q", result.Error)
	}
	if result.TxHash == "" {
		t.Error("expected non-empty TxHash on registration success")
	}
}

// TestRunOnce_RegistrationFails covers: node healthy, DID absent, registration fails.
func TestRunOnce_RegistrationFails(t *testing.T) {
	dir := t.TempDir()
	writeFakePython3(t, dir, "", 1) // exits non-zero → registration fails

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+origPath)
	defer os.Setenv("PATH", origPath)

	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": nil,
		},
	}
	d := NewDiscovererWithCaller(testCfg(), nullLogger(), mock)

	result, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error message when registration fails")
	}
	if result.Registered {
		t.Error("Registered should be false on failure")
	}
}

// ── Start (short-circuit with cancelled context) ──────────────────────────────

func TestStart_CancelledImmediately(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": "0xdeadbeef",
		},
	}
	cfg := testCfg()
	cfg.CheckInterval = 100 * time.Millisecond

	d := NewDiscovererWithCaller(cfg, nullLogger(), mock)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Give it time to run the first cycle.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

// ── Start — ticker fires ──────────────────────────────────────────────────────

// TestStart_TickFires verifies that the discovery loop actually ticks (not just
// runs once). Uses a very short interval and waits for multiple cycles.
func TestStart_TickFires(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": "0xdeadbeef",
		},
	}
	cfg := testCfg()
	cfg.CheckInterval = 30 * time.Millisecond

	d := NewDiscovererWithCaller(cfg, nullLogger(), mock)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Wait long enough for at least 2 ticks (30ms × 3 = 90ms + buffer).
	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

// TestStart_ZeroInterval verifies that a zero interval falls back to 6h
// (the loop still starts and cancels correctly).
func TestStart_ZeroInterval(t *testing.T) {
	mock := &mockRPCCaller{
		responses: map[string]interface{}{
			"system_health":    healthResult(false),
			"state_getStorage": "0xdeadbeef",
		},
	}
	cfg := testCfg()
	cfg.CheckInterval = 0 // zero → defaults to 6h inside Start

	d := NewDiscovererWithCaller(cfg, nullLogger(), mock)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		d.Start(ctx)
		close(done)
	}()

	// Cancel immediately after the first RunOnce completes.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

// ── DiscoveryResult JSON serialisation ───────────────────────────────────────

func TestDiscoveryResult_JSONRoundTrip(t *testing.T) {
	orig := &DiscoveryResult{
		AlreadyRegistered: true,
		Registered:        false,
		TxHash:            "",
		Error:             "",
	}

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got DiscoveryResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.AlreadyRegistered != orig.AlreadyRegistered {
		t.Errorf("AlreadyRegistered mismatch: got %v, want %v", got.AlreadyRegistered, orig.AlreadyRegistered)
	}
}

// ── httpCaller ────────────────────────────────────────────────────────────────
// These tests cover the default HTTP JSON-RPC transport used in production.

// TestHTTPCaller_Success verifies a healthy JSON-RPC round trip.
func TestHTTPCaller_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"isSyncing":false}}`))
	}))
	defer srv.Close()

	caller := &httpCaller{timeout: 5 * time.Second}
	resp, err := caller.Call(context.Background(), srv.URL, SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "system_health",
		Params:  []interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// TestHTTPCaller_ServerError verifies a non-200 HTTP response is an error.
func TestHTTPCaller_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	caller := &httpCaller{timeout: 5 * time.Second}
	_, err := caller.Call(context.Background(), srv.URL, SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "system_health",
	})
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

// TestHTTPCaller_BadURL verifies a network-level error is propagated.
func TestHTTPCaller_BadURL(t *testing.T) {
	caller := &httpCaller{timeout: 500 * time.Millisecond}
	_, err := caller.Call(context.Background(), "http://127.0.0.1:1", SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "system_health",
	})
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

// TestHTTPCaller_InvalidJSON verifies that malformed JSON in the response body
// results in an unmarshal error.
func TestHTTPCaller_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("NOT_JSON"))
	}))
	defer srv.Close()

	caller := &httpCaller{timeout: 5 * time.Second}
	_, err := caller.Call(context.Background(), srv.URL, SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "system_health",
	})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

// TestHTTPCaller_RPCError verifies that a well-formed JSON-RPC error response
// is returned without a transport error.
func TestHTTPCaller_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`))
	}))
	defer srv.Close()

	caller := &httpCaller{timeout: 5 * time.Second}
	resp, err := caller.Call(context.Background(), srv.URL, SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "unknown_method",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected RPC error in response")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}
