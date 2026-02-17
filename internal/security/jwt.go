package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrMissingToken is returned when no Authorization header is present.
	ErrMissingToken = errors.New("security: missing authorization token")
	// ErrInvalidToken is returned when the JWT is malformed or signature is invalid.
	ErrInvalidToken = errors.New("security: invalid token")
	// ErrExpiredToken is returned when the JWT has expired.
	ErrExpiredToken = errors.New("security: token expired")
	// ErrInsufficientRole is returned when the user's role lacks permission.
	ErrInsufficientRole = errors.New("security: insufficient role")
)

type contextKey string

const claimsKey contextKey = "jwt_claims"

// Claims represents the JWT claims for EvoClaw API authentication.
type Claims struct {
	AgentID   string `json:"agent_id"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// jwtClaims wraps Claims for jwt-go compatibility.
type jwtClaims struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the given agent and role.
func GenerateToken(agentID, role string, secret []byte, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		AgentID: agentID,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ValidateToken parses and validates a JWT string, returning the claims.
func ValidateToken(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	jc, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return &Claims{
		AgentID:   jc.AgentID,
		Role:      jc.Role,
		IssuedAt:  jc.IssuedAt.Unix(),
		ExpiresAt: jc.ExpiresAt.Unix(),
	}, nil
}

// GetClaims extracts JWT claims from the request context.
func GetClaims(r *http.Request) (*Claims, error) {
	claims, ok := r.Context().Value(claimsKey).(*Claims)
	if !ok || claims == nil {
		return nil, ErrMissingToken
	}
	return claims, nil
}

// GetJWTSecret returns the JWT secret from environment or empty (dev mode).
func GetJWTSecret() []byte {
	s := os.Getenv("EVOCLAW_JWT_SECRET")
	if s == "" {
		return nil
	}
	return []byte(s)
}

// AuthMiddleware returns HTTP middleware that validates JWT Bearer tokens.
// If secret is nil, dev mode is enabled (all requests pass through unauthenticated).
func AuthMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == nil {
				slog.Warn("JWT authentication disabled (dev mode): EVOCLAW_JWT_SECRET not set")
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ValidateToken(parts[1], secret)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
