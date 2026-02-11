//! Cryptographic signing and verification for genome constraints (Security Layer 1).
//!
//! Mirrors the Go `internal/security` package: Ed25519 signatures over
//! deterministic JSON of GenomeConstraints.

use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use serde_json;

use crate::genome::GenomeConstraints;

/// Errors from constraint signing/verification.
#[derive(Debug, thiserror::Error)]
pub enum SecurityError {
    #[error("invalid constraint signature")]
    InvalidSignature,
    #[error("missing constraint signature")]
    MissingSignature,
    #[error("missing owner public key")]
    MissingPublicKey,
    #[error("serialization error: {0}")]
    Serialization(String),
    #[error("key error: {0}")]
    KeyError(String),
}

/// Generate a new Ed25519 key pair. Returns (public_key_bytes, secret_key_bytes).
pub fn generate_owner_keypair() -> Result<([u8; 32], [u8; 32]), SecurityError> {
    let mut csprng = rand::thread_rng();
    let signing_key = SigningKey::generate(&mut csprng);
    let verifying_key = signing_key.verifying_key();
    Ok((verifying_key.to_bytes(), signing_key.to_bytes()))
}

/// Deterministic JSON serialization of constraints, matching the Go side.
pub fn serialize_constraints(c: &GenomeConstraints) -> Result<Vec<u8>, SecurityError> {
    // Build sorted representation matching Go's SerializeConstraints
    let mut allowed = c.allowed_assets.clone();
    allowed.sort();
    let mut blocked = c.blocked_actions.clone();
    blocked.sort();

    let ordered = serde_json::json!({
        "allowed_assets": allowed,
        "blocked_actions": blocked,
        "max_divergence": c.max_divergence,
        "max_loss_usd": c.max_loss_usd,
        "min_vfm_score": c.min_vfm_score,
    });
    serde_json::to_vec(&ordered).map_err(|e| SecurityError::Serialization(e.to_string()))
}

/// Sign constraints with the owner's secret key bytes (32 bytes).
pub fn sign_constraints(
    c: &GenomeConstraints,
    secret_key_bytes: &[u8; 32],
) -> Result<Vec<u8>, SecurityError> {
    let signing_key = SigningKey::from_bytes(secret_key_bytes);
    let msg = serialize_constraints(c)?;
    let sig = signing_key.sign(&msg);
    Ok(sig.to_bytes().to_vec())
}

/// Verify a constraint signature against the owner's public key bytes (32 bytes).
pub fn verify_constraints(
    c: &GenomeConstraints,
    signature: &[u8],
    public_key_bytes: &[u8],
) -> Result<bool, SecurityError> {
    if public_key_bytes.len() != 32 {
        return Err(SecurityError::MissingPublicKey);
    }
    if signature.is_empty() {
        return Err(SecurityError::MissingSignature);
    }

    let pk_bytes: [u8; 32] = public_key_bytes
        .try_into()
        .map_err(|_| SecurityError::KeyError("invalid public key length".into()))?;
    let verifying_key =
        VerifyingKey::from_bytes(&pk_bytes).map_err(|e| SecurityError::KeyError(e.to_string()))?;

    let sig_bytes: [u8; 64] = signature
        .try_into()
        .map_err(|_| SecurityError::InvalidSignature)?;
    let sig = Signature::from_bytes(&sig_bytes);

    let msg = serialize_constraints(c)?;
    Ok(verifying_key.verify(&msg, &sig).is_ok())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_keygen() {
        let (pk, sk) = generate_owner_keypair().unwrap();
        assert_eq!(pk.len(), 32);
        assert_eq!(sk.len(), 32);
    }

    #[test]
    fn test_sign_verify_roundtrip() {
        let (pk, sk) = generate_owner_keypair().unwrap();
        let c = GenomeConstraints {
            max_loss_usd: 500.0,
            blocked_actions: vec!["sell_all".into()],
            allowed_assets: vec!["BTC".into(), "ETH".into()],
            max_divergence: 10.0,
            min_vfm_score: 0.5,
        };
        let sig = sign_constraints(&c, &sk).unwrap();
        assert!(verify_constraints(&c, &sig, &pk).unwrap());
    }

    #[test]
    fn test_tampered_rejected() {
        let (pk, sk) = generate_owner_keypair().unwrap();
        let c = GenomeConstraints {
            max_loss_usd: 500.0,
            ..Default::default()
        };
        let sig = sign_constraints(&c, &sk).unwrap();

        let mut tampered = c.clone();
        tampered.max_loss_usd = 999999.0;
        assert!(!verify_constraints(&tampered, &sig, &pk).unwrap());
    }

    #[test]
    fn test_wrong_key_rejected() {
        let (_, sk1) = generate_owner_keypair().unwrap();
        let (pk2, _) = generate_owner_keypair().unwrap();
        let c = GenomeConstraints::default();
        let sig = sign_constraints(&c, &sk1).unwrap();
        assert!(!verify_constraints(&c, &sig, &pk2).unwrap());
    }

    #[test]
    fn test_deterministic_serialization() {
        let c = GenomeConstraints {
            max_loss_usd: 100.0,
            blocked_actions: vec!["b".into(), "a".into()],
            allowed_assets: vec!["ETH".into(), "BTC".into()],
            ..Default::default()
        };
        let b1 = serialize_constraints(&c).unwrap();
        let b2 = serialize_constraints(&c).unwrap();
        assert_eq!(b1, b2);
    }
}
