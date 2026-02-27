//! Security module: constraint signing (Layer 1) and JWT authentication (Layer 2).
//!
//! Layer 1: Ed25519 signatures over deterministic JSON of GenomeConstraints.
//! Layer 2: JWT validation for API tokens received from the hub.

#![allow(dead_code)]

use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use jsonwebtoken::{decode, DecodingKey, TokenData, Validation};
use serde::{Deserialize, Serialize};

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
    #[error("invalid JWT: {0}")]
    InvalidJwt(String),
    #[error("expired JWT")]
    ExpiredJwt,
    #[error("insufficient role")]
    InsufficientRole,
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

// ─── JWT Authentication (Security Layer 2) ───────────────────────────

/// JWT claims for EvoClaw API tokens.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct JwtClaims {
    pub agent_id: String,
    pub role: String,
    pub iat: i64,
    pub exp: i64,
}

/// Validate a JWT token string using the given HMAC secret.
/// Returns the decoded claims on success.
pub fn validate_jwt(token: &str, secret: &[u8]) -> Result<JwtClaims, SecurityError> {
    let key = DecodingKey::from_secret(secret);
    let validation = Validation::new(jsonwebtoken::Algorithm::HS256);

    let token_data: TokenData<JwtClaims> =
        decode(token, &key, &validation).map_err(|e| match e.kind() {
            jsonwebtoken::errors::ErrorKind::ExpiredSignature => SecurityError::ExpiredJwt,
            _ => SecurityError::InvalidJwt(e.to_string()),
        })?;

    Ok(token_data.claims)
}

/// Check if a role is allowed to access the given method + path.
/// Owner has full access, agent has limited access, readonly is GET-only.
pub fn check_permission(role: &str, method: &str, path: &str) -> bool {
    match role {
        "owner" => true,
        "agent" => {
            (method == "GET" && (path.contains("/genome") || path.contains("/genome/behavior")))
                || (method == "POST" && path.contains("/feedback"))
        }
        "readonly" => method == "GET" && path.starts_with("/api/"),
        _ => false,
    }
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

    // ─── JWT Tests ───────────────────────────────────────────────

    use jsonwebtoken::{encode, EncodingKey, Header};

    fn make_token(agent_id: &str, role: &str, secret: &[u8], exp_offset: i64) -> String {
        let now = chrono::Utc::now().timestamp();
        let claims = JwtClaims {
            agent_id: agent_id.to_string(),
            role: role.to_string(),
            iat: now,
            exp: now + exp_offset,
        };
        encode(
            &Header::default(),
            &claims,
            &EncodingKey::from_secret(secret),
        )
        .unwrap()
    }

    #[test]
    fn test_jwt_valid() {
        let secret = b"test-secret-key";
        let token = make_token("agent-1", "owner", secret, 3600);
        let claims = validate_jwt(&token, secret).unwrap();
        assert_eq!(claims.agent_id, "agent-1");
        assert_eq!(claims.role, "owner");
    }

    #[test]
    fn test_jwt_expired() {
        let secret = b"test-secret-key";
        let token = make_token("agent-1", "owner", secret, -3600);
        match validate_jwt(&token, secret) {
            Err(SecurityError::ExpiredJwt) => {}
            other => panic!("expected ExpiredJwt, got {:?}", other),
        }
    }

    #[test]
    fn test_jwt_wrong_secret() {
        let token = make_token("agent-1", "owner", b"secret-1", 3600);
        match validate_jwt(&token, b"secret-2") {
            Err(SecurityError::InvalidJwt(_)) => {}
            other => panic!("expected InvalidJwt, got {:?}", other),
        }
    }

    #[test]
    fn test_jwt_invalid_token() {
        match validate_jwt("not-a-jwt", b"secret") {
            Err(SecurityError::InvalidJwt(_)) => {}
            other => panic!("expected InvalidJwt, got {:?}", other),
        }
    }

    #[test]
    fn test_check_permission_owner() {
        assert!(check_permission("owner", "GET", "/api/status"));
        assert!(check_permission("owner", "POST", "/api/agents/a1/feedback"));
        assert!(check_permission("owner", "DELETE", "/api/agents/a1"));
    }

    #[test]
    fn test_check_permission_agent() {
        assert!(check_permission("agent", "GET", "/api/agents/a1/genome"));
        assert!(check_permission(
            "agent",
            "GET",
            "/api/agents/a1/genome/behavior"
        ));
        assert!(check_permission("agent", "POST", "/api/agents/a1/feedback"));
        assert!(!check_permission("agent", "PUT", "/api/agents/a1/genome"));
        assert!(!check_permission("agent", "DELETE", "/api/agents/a1"));
    }

    #[test]
    fn test_check_permission_readonly() {
        assert!(check_permission("readonly", "GET", "/api/status"));
        assert!(!check_permission(
            "readonly",
            "POST",
            "/api/agents/a1/feedback"
        ));
        assert!(!check_permission(
            "readonly",
            "PUT",
            "/api/agents/a1/genome"
        ));
    }
}
