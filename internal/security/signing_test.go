package security

import (
	"crypto/ed25519"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestGenerateOwnerKeyPair(t *testing.T) {
	pub, priv, err := GenerateOwnerKeyPair()
	if err != nil {
		t.Fatalf("GenerateOwnerKeyPair: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key length = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("private key length = %d, want %d", len(priv), ed25519.PrivateKeySize)
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv, err := GenerateOwnerKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	constraints := config.GenomeConstraints{
		MaxLossUSD:     500.0,
		BlockedActions: []string{"sell_all", "margin_trade"},
		AllowedAssets:  []string{"BTC", "ETH"},
		MaxDivergence:  10.0,
		MinVFMScore:    0.5,
	}

	sig, err := SignConstraints(constraints, priv)
	if err != nil {
		t.Fatalf("SignConstraints: %v", err)
	}

	ok, err := VerifyConstraints(constraints, sig, pub)
	if err != nil {
		t.Fatalf("VerifyConstraints: %v", err)
	}
	if !ok {
		t.Fatal("expected valid signature")
	}
}

func TestTamperedConstraintsRejected(t *testing.T) {
	pub, priv, err := GenerateOwnerKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	original := config.GenomeConstraints{
		MaxLossUSD:     500.0,
		BlockedActions: []string{"sell_all"},
	}

	sig, err := SignConstraints(original, priv)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with constraints
	tampered := original
	tampered.MaxLossUSD = 999999.0

	ok, err := VerifyConstraints(tampered, sig, pub)
	if err != nil {
		t.Fatalf("VerifyConstraints: %v", err)
	}
	if ok {
		t.Fatal("expected tampered constraints to be rejected")
	}
}

func TestMissingSignature(t *testing.T) {
	pub, _, err := GenerateOwnerKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	c := config.GenomeConstraints{MaxLossUSD: 100}
	_, err = VerifyConstraints(c, nil, pub)
	if err != ErrMissingSignature {
		t.Fatalf("expected ErrMissingSignature, got %v", err)
	}
}

func TestMissingPublicKey(t *testing.T) {
	c := config.GenomeConstraints{MaxLossUSD: 100}
	_, err := VerifyConstraints(c, []byte("fake"), nil)
	if err != ErrMissingPublicKey {
		t.Fatalf("expected ErrMissingPublicKey, got %v", err)
	}
}

func TestWrongKeyRejected(t *testing.T) {
	_, priv1, _ := GenerateOwnerKeyPair()
	pub2, _, _ := GenerateOwnerKeyPair()

	c := config.GenomeConstraints{MaxLossUSD: 100}
	sig, _ := SignConstraints(c, priv1)

	ok, err := VerifyConstraints(c, sig, pub2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected wrong key to be rejected")
	}
}

func TestDeterministicSerialization(t *testing.T) {
	c := config.GenomeConstraints{
		MaxLossUSD:     100,
		BlockedActions: []string{"b", "a"},
		AllowedAssets:  []string{"ETH", "BTC"},
	}

	b1, _ := SerializeConstraints(c)
	b2, _ := SerializeConstraints(c)

	if string(b1) != string(b2) {
		t.Fatalf("serialization not deterministic:\n%s\n%s", b1, b2)
	}
}

func TestUnsignedConstraintsBackwardCompat(t *testing.T) {
	// Unsigned constraints (empty sig + empty key) should return specific errors,
	// allowing callers to decide whether to warn or reject.
	c := config.GenomeConstraints{MaxLossUSD: 100}

	// No public key → ErrMissingPublicKey
	_, err := VerifyConstraints(c, nil, nil)
	if err != ErrMissingPublicKey {
		t.Fatalf("expected ErrMissingPublicKey for nil key, got %v", err)
	}
}
