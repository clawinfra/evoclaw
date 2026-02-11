package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := []byte("test-secret-key-32bytes-long!!!!!")
	token, err := GenerateToken("agent-1", RoleOwner, secret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", claims.AgentID, "agent-1")
	}
	if claims.Role != RoleOwner {
		t.Errorf("Role = %q, want %q", claims.Role, RoleOwner)
	}
	if claims.IssuedAt == 0 {
		t.Error("IssuedAt should be set")
	}
	if claims.ExpiresAt == 0 {
		t.Error("ExpiresAt should be set")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken("agent-1", RoleOwner, secret, -time.Hour)
	_, err := ValidateToken(token, secret)
	if err != ErrExpiredToken {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestInvalidTokenRejected(t *testing.T) {
	secret := []byte("test-secret")
	_, err := ValidateToken("not-a-valid-jwt", secret)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestWrongSecretRejected(t *testing.T) {
	secret1 := []byte("secret-1")
	secret2 := []byte("secret-2")
	token, _ := GenerateToken("agent-1", RoleOwner, secret1, time.Hour)
	_, err := ValidateToken(token, secret2)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	secret := []byte("test-secret")
	handler := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	secret := []byte("test-secret")
	handler := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken("agent-1", RoleOwner, secret, time.Hour)

	var gotClaims *Claims
	handler := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = GetClaims(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotClaims == nil || gotClaims.AgentID != "agent-1" {
		t.Fatal("claims not set in context")
	}
}

func TestAuthMiddleware_DevMode(t *testing.T) {
	// nil secret = dev mode, should pass through
	handler := AuthMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d", w.Code)
	}
}

func TestGetClaims_NoClaims(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	_, err := GetClaims(req)
	if err != ErrMissingToken {
		t.Fatalf("expected ErrMissingToken, got %v", err)
	}
}

func TestAuthMiddleware_BadAuthHeader(t *testing.T) {
	secret := []byte("test-secret")
	handler := AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// Test RBAC integration with auth middleware
func TestAuthMiddleware_RBAC_OwnerFullAccess(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken("owner-1", RoleOwner, secret, time.Hour)

	handler := AuthMiddleware(secret)(RequireRole(RoleOwner)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		req := httptest.NewRequest(method, "/api/agents/x/genome", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("owner %s: expected 200, got %d", method, w.Code)
		}
	}
}

func TestAuthMiddleware_RBAC_ReadonlyBlocked(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken("reader-1", RoleReadonly, secret, time.Hour)

	handler := AuthMiddleware(secret)(RequireRole(RoleOwner, RoleAgent)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("POST", "/api/agents/x/feedback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for readonly POST, got %d", w.Code)
	}
}

func TestAuthMiddleware_RBAC_AgentLimited(t *testing.T) {
	secret := []byte("test-secret")
	token, _ := GenerateToken("agent-1", RoleAgent, secret, time.Hour)

	// Agent should be blocked from owner-only endpoints
	handler := AuthMiddleware(secret)(RequireRole(RoleOwner)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("PUT", "/api/agents/x/genome/constraints", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for agent PUT constraints, got %d", w.Code)
	}
}
