// Package clawchain provides ClawChain Substrate node integration for EvoClaw.
// This file implements Substrate storage key computation in pure Go,
// with no external dependencies beyond stdlib and golang.org/x/crypto/blake2b.
package clawchain

import (
	"encoding/binary"
	"encoding/hex"
	"math/bits"

	"golang.org/x/crypto/blake2b"
)

// xxHash64 primes from the xxHash spec (https://github.com/Cyan4973/xxHash/blob/v0.8.1/doc/xxhash_spec.md).
const (
	xxPrime1 uint64 = 11400714785074694791
	xxPrime2 uint64 = 14029467366897019727
	xxPrime3 uint64 = 1609587929392839161
	xxPrime4 uint64 = 9650029242287828579
	xxPrime5 uint64 = 2870177450012600261
)

// xxRound processes one 8-byte lane of input.
func xxRound(acc, input uint64) uint64 {
	acc += input * xxPrime2
	acc = bits.RotateLeft64(acc, 31)
	acc *= xxPrime1
	return acc
}

// xxMergeRound merges an accumulator lane into the hash state.
func xxMergeRound(acc, val uint64) uint64 {
	val = xxRound(0, val)
	acc ^= val
	acc = acc*xxPrime1 + xxPrime4
	return acc
}

// xxHash64 implements xxHash-64 with the given seed.
//
// Reference: https://github.com/Cyan4973/xxHash/blob/v0.8.1/doc/xxhash_spec.md
// This is used internally by TwoX128 to implement Substrate's TwoX128 storage hasher.
func xxHash64(input []byte, seed uint64) uint64 {
	n := len(input)
	var h uint64

	if n >= 32 {
		v1 := seed + xxPrime1 + xxPrime2
		v2 := seed + xxPrime2
		v3 := seed
		v4 := seed - xxPrime1

		for len(input) >= 32 {
			v1 = xxRound(v1, binary.LittleEndian.Uint64(input[0:8]))
			v2 = xxRound(v2, binary.LittleEndian.Uint64(input[8:16]))
			v3 = xxRound(v3, binary.LittleEndian.Uint64(input[16:24]))
			v4 = xxRound(v4, binary.LittleEndian.Uint64(input[24:32]))
			input = input[32:]
		}

		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) +
			bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = xxMergeRound(h, v1)
		h = xxMergeRound(h, v2)
		h = xxMergeRound(h, v3)
		h = xxMergeRound(h, v4)
	} else {
		h = seed + xxPrime5
	}

	h += uint64(n)

	// Process remaining 8-byte chunks.
	for len(input) >= 8 {
		k1 := xxRound(0, binary.LittleEndian.Uint64(input[:8]))
		h ^= k1
		h = bits.RotateLeft64(h, 27)*xxPrime1 + xxPrime4
		input = input[8:]
	}

	// Process remaining 4-byte chunk.
	if len(input) >= 4 {
		h ^= uint64(binary.LittleEndian.Uint32(input[:4])) * xxPrime1
		h = bits.RotateLeft64(h, 23)*xxPrime2 + xxPrime3
		input = input[4:]
	}

	// Process remaining bytes one at a time.
	for _, b := range input {
		h ^= uint64(b) * xxPrime5
		h = bits.RotateLeft64(h, 11) * xxPrime1
	}

	// Final avalanche mix.
	h ^= h >> 33
	h *= xxPrime2
	h ^= h >> 29
	h *= xxPrime3
	h ^= h >> 32

	return h
}

// TwoX128 computes the TwoX128 hash of data using two 64-bit xxHash calls
// (seeds 0 and 1) and returns the 16-byte result in little-endian order.
//
// This is Substrate's default hasher for module and storage prefixes. The
// output is NOT cryptographically secure; use Blake2 variants for map keys.
//
// Example (from Substrate documentation):
//
//	TwoX128([]byte("System")) == 0x26aa394eea5630e07c48ae0c9558cef7
func TwoX128(data []byte) []byte {
	h0 := xxHash64(data, 0)
	h1 := xxHash64(data, 1)
	out := make([]byte, 16)
	binary.LittleEndian.PutUint64(out[0:8], h0)
	binary.LittleEndian.PutUint64(out[8:16], h1)
	return out
}

// Blake2_128Concat computes the 16-byte BLAKE2b-128 digest of key, then
// appends the raw key bytes. This is Substrate's Blake2_128Concat storage
// hasher, used for map keys that require both collision resistance and
// transparent iteration over the original key.
func Blake2_128Concat(key []byte) []byte {
	h, _ := blake2b.New(16, nil) // 16-byte output; error is only for invalid key size
	h.Write(key)
	sum := h.Sum(nil) // 16 bytes
	return append(sum, key...)
}

// ComputeStorageKey computes the full Substrate storage key for a map entry:
//
//	TwoX128(module) ++ TwoX128(storage) ++ Blake2_128Concat(key)
//
// The result is returned as a "0x"-prefixed hex string, ready to pass to
// the state_getStorage JSON-RPC method.
//
// Parameters:
//   - module:  pallet name, e.g. "AgentDid"
//   - storage: storage item name, e.g. "DIDDocuments"
//   - key:     raw map key bytes (e.g. 32-byte SS58-decoded account ID)
func ComputeStorageKey(module, storage string, key []byte) string {
	prefix := append(TwoX128([]byte(module)), TwoX128([]byte(storage))...)
	suffix := Blake2_128Concat(key)
	full := append(prefix, suffix...)
	return "0x" + hex.EncodeToString(full)
}
