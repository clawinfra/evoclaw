//! Native Rust EIP-712 signing for Hyperliquid.
//!
//! Replaces the Python dependency (scripts/hl_sign.py) with a fully native
//! implementation using alloy and k256 crates.
//!
//! Hyperliquid uses two signing schemes:
//! 1. **L1 actions** (orders, cancels, etc.): msgpack-hash the action → wrap in
//!    a "phantom agent" EIP-712 struct → sign.
//! 2. **User-signed actions** (transfers, withdrawals, etc.): direct EIP-712
//!    typed-data signing.
//!
//! This module implements scheme #1 which is what trading operations need.

use alloy_primitives::{Address, FixedBytes};
use alloy_signer::Signer;
use alloy_signer_local::PrivateKeySigner;
use alloy_sol_types::{eip712_domain, sol, SolStruct};
use serde::{Deserialize, Serialize};
use tiny_keccak::{Hasher, Keccak};
use tracing::debug;

use crate::config::NetworkMode;

/// Signature components
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EthSignature {
    pub r: String,
    pub s: String,
    pub v: u8,
}

/// EIP-712 domain for Hyperliquid Exchange
fn exchange_domain() -> alloy_sol_types::Eip712Domain {
    eip712_domain! {
        name: "Exchange",
        version: "1",
        chain_id: 1337,
        verifying_contract: Address::ZERO,
    }
}

// Define the Agent struct used for L1 action signing
sol! {
    #[derive(Debug, serde::Serialize, serde::Deserialize)]
    struct Agent {
        string source;
        bytes32 connectionId;
    }
}

/// Compute the keccak256 hash of the action payload.
///
/// Following the Python SDK: msgpack(action) + nonce(8 bytes big-endian)
/// + vault_indicator + optional expires_after
pub fn action_hash(
    action: &serde_json::Value,
    vault_address: Option<&str>,
    nonce: u64,
    expires_after: Option<u64>,
) -> [u8; 32] {
    let packed = rmp_serde::to_vec_named(action).expect("msgpack serialization should not fail");

    let mut data = packed;
    data.extend_from_slice(&nonce.to_be_bytes());

    match vault_address {
        None => {
            data.push(0x00);
        }
        Some(addr) => {
            data.push(0x01);
            let addr_bytes = hex_to_bytes(addr);
            data.extend_from_slice(&addr_bytes);
        }
    }

    if let Some(expires) = expires_after {
        data.push(0x00);
        data.extend_from_slice(&expires.to_be_bytes());
    }

    keccak256(&data)
}

/// Sign an L1 action (orders, cancels, modifications, etc.)
///
/// This follows the official Hyperliquid signing flow:
/// 1. msgpack-serialize the action
/// 2. Append nonce + vault info
/// 3. keccak256 hash the payload
/// 4. Wrap in a "phantom agent" EIP-712 message
/// 5. Sign with EIP-712 typed data signing
pub async fn sign_l1_action(
    private_key_hex: &str,
    action: &serde_json::Value,
    vault_address: Option<&str>,
    nonce: u64,
    expires_after: Option<u64>,
    network: NetworkMode,
) -> Result<EthSignature, Box<dyn std::error::Error>> {
    let hash = action_hash(action, vault_address, nonce, expires_after);
    let connection_id = FixedBytes::from(hash);

    let source = network.source_id().to_string();

    let phantom_agent = Agent {
        source: source.clone(),
        connectionId: connection_id,
    };

    let domain = exchange_domain();

    // Compute the EIP-712 signing hash
    let signing_hash = phantom_agent.eip712_signing_hash(&domain);

    // Parse private key and sign
    let signer: PrivateKeySigner = private_key_hex.parse()?;
    let signature = signer.sign_hash(&signing_hash).await?;

    let sig_bytes = signature.as_bytes();
    // alloy Signature.as_bytes() returns 65 bytes: r(32) + s(32) + v(1)
    let r_bytes = &sig_bytes[..32];
    let s_bytes = &sig_bytes[32..64];
    let v = sig_bytes[64];

    debug!(
        source = %source,
        nonce = nonce,
        "signed L1 action"
    );

    Ok(EthSignature {
        r: format!("0x{}", hex::encode(r_bytes)),
        s: format!("0x{}", hex::encode(s_bytes)),
        v: if v < 27 { v + 27 } else { v },
    })
}

/// Compute keccak256 hash
fn keccak256(data: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak::v256();
    let mut output = [0u8; 32];
    hasher.update(data);
    hasher.finalize(&mut output);
    output
}

/// Convert a hex address string (with or without 0x prefix) to bytes
fn hex_to_bytes(hex_str: &str) -> Vec<u8> {
    let s = hex_str.strip_prefix("0x").unwrap_or(hex_str);
    hex::decode(s).expect("invalid hex string")
}

/// Load a private key from a file
pub fn load_private_key(path: &str) -> Result<String, Box<dyn std::error::Error>> {
    let key = std::fs::read_to_string(path)
        .map_err(|e| format!("failed to read private key from {}: {}", path, e))?
        .trim()
        .to_string();
    Ok(key)
}

/// Derive the wallet address from a private key
pub fn derive_address(private_key_hex: &str) -> Result<String, Box<dyn std::error::Error>> {
    let signer: PrivateKeySigner = private_key_hex.parse()?;
    Ok(format!("{:?}", signer.address()))
}

#[cfg(test)]
mod tests {
    use super::*;

    // A well-known test private key (DO NOT use on mainnet)
    const TEST_PRIVATE_KEY: &str =
        "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80";

    #[test]
    fn test_keccak256_empty() {
        let hash = keccak256(b"");
        let expected = "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470";
        assert_eq!(hex::encode(hash), expected);
    }

    #[test]
    fn test_keccak256_hello() {
        let hash = keccak256(b"hello");
        let expected = "1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8";
        assert_eq!(hex::encode(hash), expected);
    }

    #[test]
    fn test_hex_to_bytes_with_prefix() {
        let bytes = hex_to_bytes("0xabcdef");
        assert_eq!(bytes, vec![0xab, 0xcd, 0xef]);
    }

    #[test]
    fn test_hex_to_bytes_without_prefix() {
        let bytes = hex_to_bytes("abcdef");
        assert_eq!(bytes, vec![0xab, 0xcd, 0xef]);
    }

    #[test]
    fn test_action_hash_deterministic() {
        let action = serde_json::json!({
            "type": "order",
            "orders": [{"a": 0, "b": true, "p": "50000", "s": "0.1", "r": false, "t": {"limit": {"tif": "Gtc"}}}],
            "grouping": "na"
        });

        let hash1 = action_hash(&action, None, 1000, None);
        let hash2 = action_hash(&action, None, 1000, None);
        assert_eq!(hash1, hash2);
    }

    #[test]
    fn test_action_hash_different_nonces() {
        let action = serde_json::json!({"type": "order"});

        let hash1 = action_hash(&action, None, 1000, None);
        let hash2 = action_hash(&action, None, 2000, None);
        assert_ne!(hash1, hash2);
    }

    #[test]
    fn test_action_hash_with_vault() {
        let action = serde_json::json!({"type": "order"});

        let hash_no_vault = action_hash(&action, None, 1000, None);
        let hash_with_vault = action_hash(
            &action,
            Some("0x0000000000000000000000000000000000000001"),
            1000,
            None,
        );
        assert_ne!(hash_no_vault, hash_with_vault);
    }

    #[test]
    fn test_action_hash_with_expires() {
        let action = serde_json::json!({"type": "order"});

        let hash_no_expires = action_hash(&action, None, 1000, None);
        let hash_with_expires = action_hash(&action, None, 1000, Some(5000));
        assert_ne!(hash_no_expires, hash_with_expires);
    }

    #[tokio::test]
    async fn test_sign_l1_action_produces_valid_signature() {
        let action = serde_json::json!({
            "type": "order",
            "orders": [{
                "a": 0,
                "b": true,
                "p": "50000.0",
                "s": "0.01",
                "r": false,
                "t": {"limit": {"tif": "Gtc"}}
            }],
            "grouping": "na"
        });

        let result = sign_l1_action(
            TEST_PRIVATE_KEY,
            &action,
            None,
            1234567890,
            None,
            NetworkMode::Testnet,
        )
        .await;

        assert!(result.is_ok());
        let sig = result.unwrap();
        assert!(sig.r.starts_with("0x"));
        assert!(sig.s.starts_with("0x"));
        assert!(sig.v == 27 || sig.v == 28);
        // r and s should be 32 bytes = 64 hex chars + "0x" prefix
        assert_eq!(sig.r.len(), 66);
        assert_eq!(sig.s.len(), 66);
    }

    #[tokio::test]
    async fn test_sign_l1_action_deterministic() {
        let action = serde_json::json!({"type": "cancel", "cancels": [{"a": 0, "o": 12345}]});

        let sig1 = sign_l1_action(
            TEST_PRIVATE_KEY,
            &action,
            None,
            1000,
            None,
            NetworkMode::Testnet,
        )
        .await
        .unwrap();

        let sig2 = sign_l1_action(
            TEST_PRIVATE_KEY,
            &action,
            None,
            1000,
            None,
            NetworkMode::Testnet,
        )
        .await
        .unwrap();

        assert_eq!(sig1.r, sig2.r);
        assert_eq!(sig1.s, sig2.s);
        assert_eq!(sig1.v, sig2.v);
    }

    #[tokio::test]
    async fn test_sign_l1_action_different_networks() {
        let action = serde_json::json!({"type": "order", "orders": [], "grouping": "na"});

        let sig_testnet = sign_l1_action(
            TEST_PRIVATE_KEY,
            &action,
            None,
            1000,
            None,
            NetworkMode::Testnet,
        )
        .await
        .unwrap();

        let sig_mainnet = sign_l1_action(
            TEST_PRIVATE_KEY,
            &action,
            None,
            1000,
            None,
            NetworkMode::Mainnet,
        )
        .await
        .unwrap();

        // Different networks should produce different signatures
        assert_ne!(sig_testnet.r, sig_mainnet.r);
    }

    #[tokio::test]
    async fn test_sign_l1_action_invalid_key() {
        let action = serde_json::json!({"type": "order"});

        let result =
            sign_l1_action("not_a_valid_key", &action, None, 1000, None, NetworkMode::Testnet)
                .await;

        assert!(result.is_err());
    }

    #[test]
    fn test_derive_address() {
        let addr = derive_address(TEST_PRIVATE_KEY).unwrap();
        // This is the well-known address for this Hardhat test key
        assert_eq!(
            addr.to_lowercase(),
            "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"
        );
    }

    #[test]
    fn test_derive_address_invalid_key() {
        let result = derive_address("not_a_key");
        assert!(result.is_err());
    }

    #[test]
    fn test_load_private_key_nonexistent() {
        let result = load_private_key("/nonexistent/key.txt");
        assert!(result.is_err());
    }

    #[test]
    fn test_load_private_key_from_file() {
        let dir = tempfile::tempdir().unwrap();
        let key_path = dir.path().join("test.key");
        std::fs::write(&key_path, "  0xdeadbeef1234  \n").unwrap();

        let key = load_private_key(key_path.to_str().unwrap()).unwrap();
        assert_eq!(key, "0xdeadbeef1234");
    }

    #[test]
    fn test_eth_signature_serialization() {
        let sig = EthSignature {
            r: "0xabc123".to_string(),
            s: "0xdef456".to_string(),
            v: 27,
        };

        let json = serde_json::to_string(&sig).unwrap();
        let deserialized: EthSignature = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.r, "0xabc123");
        assert_eq!(deserialized.s, "0xdef456");
        assert_eq!(deserialized.v, 27);
    }

    #[test]
    fn test_exchange_domain() {
        let domain = exchange_domain();
        // Just verify it constructs without panic
        let _ = domain;
    }
}
