package clawchain

import (
	"encoding/hex"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// ── xxHash64 ─────────────────────────────────────────────────────────────────

// TestXXHash64_EmptyString verifies the well-known xxHash64 test vector for
// an empty input with seed 0. This is from the xxHash spec / reference impl.
func TestXXHash64_EmptyString(t *testing.T) {
	got := xxHash64([]byte{}, 0)
	const want = uint64(0xef46db3751d8e999)
	if got != want {
		t.Errorf("xxHash64(\"\", 0) = %016x, want %016x", got, want)
	}
}

// TestXXHash64_SingleByte verifies a single-byte input against the reference.
func TestXXHash64_SingleByte(t *testing.T) {
	// "a" with seed 0 — verified against C reference implementation.
	got := xxHash64([]byte("a"), 0)
	const want = uint64(0xd24ec4f1a98c6e5b)
	if got != want {
		t.Errorf("xxHash64(\"a\", 0) = %016x, want %016x", got, want)
	}
}

// TestXXHash64_ShortInput verifies a short multi-byte input.
func TestXXHash64_ShortInput(t *testing.T) {
	// "abc" with seed 0 — verified against reference.
	got := xxHash64([]byte("abc"), 0)
	const want = uint64(0x44bc2cf5ad770999)
	if got != want {
		t.Errorf("xxHash64(\"abc\", 0) = %016x, want %016x", got, want)
	}
}

// TestXXHash64_DifferentSeeds verifies that different seeds produce different hashes.
func TestXXHash64_DifferentSeeds(t *testing.T) {
	input := []byte("hello")
	h0 := xxHash64(input, 0)
	h1 := xxHash64(input, 1)
	if h0 == h1 {
		t.Errorf("xxHash64 with seed 0 and 1 should differ for non-trivial input, got %016x for both", h0)
	}
}

// TestXXHash64_LongInput exercises the ≥32-byte (stripe) code path.
func TestXXHash64_LongInput(t *testing.T) {
	// 40-byte input crosses the 32-byte stripe boundary.
	input := []byte("01234567890123456789012345678901234567890")
	h := xxHash64(input, 0)
	// Must not be zero (collision with zero is astronomically unlikely).
	if h == 0 {
		t.Error("xxHash64 returned 0 for long input; likely a bug")
	}
	// Verify determinism.
	if xxHash64(input, 0) != h {
		t.Error("xxHash64 is not deterministic")
	}
}

// ── TwoX128 ──────────────────────────────────────────────────────────────────

// TestTwoX128_KnownValues verifies TwoX128 against values documented by
// the Substrate / Polkadot ecosystem.
//
// TwoX128("System") = 26aa394eea5630e07c48ae0c9558cef7
// (verified via Polkadot.js and substrate-storage-key tooling)
func TestTwoX128_KnownValues(t *testing.T) {
	tests := []struct {
		input string
		want  string // hex, no 0x prefix
	}{
		{
			// Widely documented in Polkadot/Substrate storage key references.
			input: "System",
			want:  "26aa394eea5630e07c48ae0c9558cef7",
		},
	}

	for _, tc := range tests {
		got := hex.EncodeToString(TwoX128([]byte(tc.input)))
		if got != tc.want {
			t.Errorf("TwoX128(%q) = %s, want %s", tc.input, got, tc.want)
		}
	}
}

// TestTwoX128_Length verifies the output is always exactly 16 bytes.
func TestTwoX128_Length(t *testing.T) {
	for _, s := range []string{"", "a", "AgentDid", "DIDDocuments", "hello world"} {
		got := TwoX128([]byte(s))
		if len(got) != 16 {
			t.Errorf("TwoX128(%q) has length %d, want 16", s, len(got))
		}
	}
}

// TestTwoX128_Deterministic verifies repeated calls give the same result.
func TestTwoX128_Deterministic(t *testing.T) {
	input := []byte("AgentDid")
	a := TwoX128(input)
	b := TwoX128(input)
	if hex.EncodeToString(a) != hex.EncodeToString(b) {
		t.Error("TwoX128 is not deterministic")
	}
}

// ── Blake2_128Concat ─────────────────────────────────────────────────────────

// TestBlake2_128Concat verifies the structure: 16-byte hash prefix + raw key.
func TestBlake2_128Concat(t *testing.T) {
	// Alice's SS58-decoded 32-byte account ID.
	aliceHex := "d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"
	alice, _ := hex.DecodeString(aliceHex)

	got := Blake2_128Concat(alice)

	// Total length = 16 (hash) + 32 (raw key) = 48 bytes.
	if len(got) != 48 {
		t.Errorf("Blake2_128Concat len = %d, want 48", len(got))
	}

	// First 16 bytes must be the BLAKE2b-128 digest of alice.
	h, _ := blake2b.New(16, nil)
	h.Write(alice)
	wantHash := h.Sum(nil)

	gotHash := got[:16]
	if hex.EncodeToString(gotHash) != hex.EncodeToString(wantHash) {
		t.Errorf("Blake2_128Concat prefix = %x, want %x", gotHash, wantHash)
	}

	// Last 32 bytes must be the raw key.
	gotKey := got[16:]
	if hex.EncodeToString(gotKey) != aliceHex {
		t.Errorf("Blake2_128Concat suffix = %x, want %s", gotKey, aliceHex)
	}
}

// TestBlake2_128Concat_EmptyKey verifies an empty key produces a 16-byte result
// (just the hash, no suffix).
func TestBlake2_128Concat_EmptyKey(t *testing.T) {
	got := Blake2_128Concat([]byte{})
	if len(got) != 16 {
		t.Errorf("Blake2_128Concat([]) len = %d, want 16", len(got))
	}
}

// ── ComputeStorageKey ─────────────────────────────────────────────────────────

// TestComputeStorageKey verifies the full storage key computation for the
// AgentDid.DIDDocuments map entry for Alice.
//
// Expected structure (hex, no 0x):
//
//	[16 bytes: TwoX128("AgentDid")] ++ [16 bytes: TwoX128("DIDDocuments")] ++
//	[16 bytes: blake2b-128(alice)] ++ [32 bytes: alice raw]
//
// Total: 80 bytes → 160 hex chars → 162 chars with "0x" prefix.
func TestComputeStorageKey(t *testing.T) {
	aliceHex := "d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"
	alice, _ := hex.DecodeString(aliceHex)

	key := ComputeStorageKey("AgentDid", "DIDDocuments", alice)

	// Must start with "0x".
	if !strings.HasPrefix(key, "0x") {
		t.Errorf("ComputeStorageKey must start with '0x', got %q", key[:min(10, len(key))])
	}

	// Decode and check total length.
	raw, err := hex.DecodeString(key[2:])
	if err != nil {
		t.Fatalf("ComputeStorageKey returned invalid hex: %v", err)
	}
	// 16 + 16 + 16 + 32 = 80 bytes
	if len(raw) != 80 {
		t.Errorf("ComputeStorageKey raw length = %d, want 80", len(raw))
	}

	// First 16 bytes = TwoX128("AgentDid").
	wantModPrefix := TwoX128([]byte("AgentDid"))
	if hex.EncodeToString(raw[:16]) != hex.EncodeToString(wantModPrefix) {
		t.Errorf("module prefix mismatch: got %x, want %x", raw[:16], wantModPrefix)
	}

	// Next 16 bytes = TwoX128("DIDDocuments").
	wantStoragePrefix := TwoX128([]byte("DIDDocuments"))
	if hex.EncodeToString(raw[16:32]) != hex.EncodeToString(wantStoragePrefix) {
		t.Errorf("storage prefix mismatch: got %x, want %x", raw[16:32], wantStoragePrefix)
	}

	// Remaining 48 bytes = Blake2_128Concat(alice).
	wantSuffix := Blake2_128Concat(alice)
	if hex.EncodeToString(raw[32:]) != hex.EncodeToString(wantSuffix) {
		t.Errorf("key suffix mismatch: got %x, want %x", raw[32:], wantSuffix)
	}
}

// TestComputeStorageKey_EmptyKey verifies the key computes cleanly with no map key.
func TestComputeStorageKey_EmptyKey(t *testing.T) {
	key := ComputeStorageKey("System", "Account", []byte{})
	if !strings.HasPrefix(key, "0x") {
		t.Error("missing 0x prefix")
	}
	raw, err := hex.DecodeString(key[2:])
	if err != nil {
		t.Fatalf("invalid hex: %v", err)
	}
	// 16 + 16 + 16 (blake2 of empty) + 0 (no suffix) = 48 bytes
	if len(raw) != 48 {
		t.Errorf("expected 48 bytes for empty key, got %d", len(raw))
	}
}

// min is a simple integer min for Go 1.21+; included here for clarity.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
