// Package security provides cryptographic signing and verification for genome constraints.
// Owner keys (Ed25519) sign constraints so the evolution engine cannot tamper with them.
package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/clawinfra/evoclaw/internal/config"
)

var (
	// ErrInvalidSignature is returned when constraint signature verification fails.
	ErrInvalidSignature = errors.New("security: invalid constraint signature")
	// ErrMissingSignature is returned when constraints are unsigned but verification is required.
	ErrMissingSignature = errors.New("security: missing constraint signature")
	// ErrMissingPublicKey is returned when the owner public key is absent.
	ErrMissingPublicKey = errors.New("security: missing owner public key")
)

// GenerateOwnerKeyPair generates a new Ed25519 key pair for signing constraints.
func GenerateOwnerKeyPair() (publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, err error) {
	publicKey, privateKey, err = ed25519.GenerateKey(rand.Reader)
	return
}

// SerializeConstraints produces a deterministic JSON representation of GenomeConstraints
// suitable for signing. Keys are sorted alphabetically.
func SerializeConstraints(c config.GenomeConstraints) ([]byte, error) {
	// Build an ordered map to guarantee deterministic output.
	m := map[string]interface{}{
		"blocked_actions": sortedStrings(c.BlockedActions),
		"allowed_assets":  sortedStrings(c.AllowedAssets),
		"max_divergence":  c.MaxDivergence,
		"max_loss_usd":    c.MaxLossUSD,
		"min_vfm_score":   c.MinVFMScore,
	}
	return deterministicJSON(m)
}

// SignConstraints signs the given constraints with the owner's private key.
func SignConstraints(c config.GenomeConstraints, privateKey ed25519.PrivateKey) ([]byte, error) {
	msg, err := SerializeConstraints(c)
	if err != nil {
		return nil, fmt.Errorf("serialize constraints for signing: %w", err)
	}
	return ed25519.Sign(privateKey, msg), nil
}

// VerifyConstraints verifies that the signature matches the constraints and public key.
func VerifyConstraints(c config.GenomeConstraints, signature []byte, publicKey ed25519.PublicKey) (bool, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return false, ErrMissingPublicKey
	}
	if len(signature) == 0 {
		return false, ErrMissingSignature
	}
	msg, err := SerializeConstraints(c)
	if err != nil {
		return false, fmt.Errorf("serialize constraints for verification: %w", err)
	}
	return ed25519.Verify(publicKey, msg, signature), nil
}

// deterministicJSON marshals a value with sorted map keys for reproducible output.
func deterministicJSON(v interface{}) ([]byte, error) {
	// encoding/json already sorts map keys when the key type is string.
	return json.Marshal(v)
}

// sortedStrings returns a sorted copy of s (nil → empty slice for consistent JSON).
func sortedStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}
