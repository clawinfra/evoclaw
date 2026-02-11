package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckPermission_Owner(t *testing.T) {
	// Owner should have access to everything
	tests := []struct {
		method, path string
	}{
		{"GET", "/api/status"},
		{"GET", "/api/agents/a1/genome"},
		{"PUT", "/api/agents/a1/genome"},
		{"POST", "/api/agents/a1/feedback"},
		{"PUT", "/api/agents/a1/genome/constraints"},
		{"DELETE", "/api/agents/a1"},
		{"GET", "/api/agents/a1/genome/behavior"},
	}
	for _, tt := range tests {
		if !CheckPermission(RoleOwner, tt.method, tt.path) {
			t.Errorf("owner should access %s %s", tt.method, tt.path)
		}
	}
}

func TestCheckPermission_Agent(t *testing.T) {
	allowed := []struct {
		method, path string
	}{
		{"GET", "/api/agents/a1/genome"},
		{"POST", "/api/agents/a1/feedback"},
		{"GET", "/api/agents/a1/genome/behavior"},
	}
	for _, tt := range allowed {
		if !CheckPermission(RoleAgent, tt.method, tt.path) {
			t.Errorf("agent should access %s %s", tt.method, tt.path)
		}
	}

	denied := []struct {
		method, path string
	}{
		{"PUT", "/api/agents/a1/genome"},
		{"PUT", "/api/agents/a1/genome/constraints"},
		{"DELETE", "/api/agents/a1"},
		{"POST", "/api/agents/a1/evolve"},
	}
	for _, tt := range denied {
		if CheckPermission(RoleAgent, tt.method, tt.path) {
			t.Errorf("agent should NOT access %s %s", tt.method, tt.path)
		}
	}
}

func TestCheckPermission_Readonly(t *testing.T) {
	allowed := []struct {
		method, path string
	}{
		{"GET", "/api/status"},
		{"GET", "/api/agents"},
		{"GET", "/api/agents/a1/genome"},
		{"GET", "/api/agents/a1/genome/behavior"},
		{"GET", "/api/models"},
	}
	for _, tt := range allowed {
		if !CheckPermission(RoleReadonly, tt.method, tt.path) {
			t.Errorf("readonly should access %s %s", tt.method, tt.path)
		}
	}

	denied := []struct {
		method, path string
	}{
		{"POST", "/api/agents/a1/feedback"},
		{"PUT", "/api/agents/a1/genome"},
		{"DELETE", "/api/agents/a1"},
		{"POST", "/api/agents/a1/evolve"},
	}
	for _, tt := range denied {
		if CheckPermission(RoleReadonly, tt.method, tt.path) {
			t.Errorf("readonly should NOT access %s %s", tt.method, tt.path)
		}
	}
}

func TestRequireRole_Middleware(t *testing.T) {
	secret := []byte("test-secret")

	tests := []struct {
		name         string
		tokenRole    string
		allowedRoles []string
		wantCode     int
	}{
		{"owner allowed", RoleOwner, []string{RoleOwner}, 200},
		{"agent allowed", RoleAgent, []string{RoleOwner, RoleAgent}, 200},
		{"readonly blocked", RoleReadonly, []string{RoleOwner, RoleAgent}, 403},
		{"agent blocked from owner-only", RoleAgent, []string{RoleOwner}, 403},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := GenerateToken("test", tt.tokenRole, secret, time.Hour)

			handler := AuthMiddleware(secret)(
				RequireRole(tt.allowedRoles...)(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
					}),
				),
			)

			req := httptest.NewRequest("GET", "/api/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestRequireRole_DevMode(t *testing.T) {
	// No claims in context (dev mode) — should pass through
	handler := RequireRole(RoleOwner)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d", w.Code)
	}
}

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"/api/", "/api/status", true},
		{"/api/agents/{id}/genome", "/api/agents/a1/genome", true},
		{"/api/agents/{id}/feedback", "/api/agents/a1/feedback", true},
		{"/api/agents/{id}/genome/behavior", "/api/agents/a1/genome/behavior", true},
		{"/api/agents/{id}/genome", "/api/status", false},
	}
	for _, tt := range tests {
		got := matchRoute(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchRoute(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
