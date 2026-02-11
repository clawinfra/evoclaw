package security

import (
	"fmt"
	"net/http"
	"strings"
)

// Roles
const (
	RoleOwner    = "owner"
	RoleAgent    = "agent"
	RoleReadonly = "readonly"
)

// ValidRoles lists all valid roles.
var ValidRoles = []string{RoleOwner, RoleAgent, RoleReadonly}

// routePermission defines which roles can access a method+path pattern.
type routePermission struct {
	Method  string // HTTP method ("GET", "POST", "PUT", "DELETE", "*" for any)
	Pattern string // path prefix or exact match
	Roles   []string
}

// permissions defines the RBAC permission table.
var permissions = []routePermission{
	// Owner: all endpoints (handled as fallback)
	// Agent-specific endpoints
	{Method: "GET", Pattern: "/api/agents/{id}/genome", Roles: []string{RoleOwner, RoleAgent, RoleReadonly}},
	{Method: "GET", Pattern: "/api/agents/{id}/genome/behavior", Roles: []string{RoleOwner, RoleAgent, RoleReadonly}},
	{Method: "POST", Pattern: "/api/agents/{id}/feedback", Roles: []string{RoleOwner, RoleAgent}},
	// All GET endpoints are available to readonly
	{Method: "GET", Pattern: "/api/", Roles: []string{RoleOwner, RoleAgent, RoleReadonly}},
	// All other methods on /api/ require owner
	{Method: "*", Pattern: "/api/", Roles: []string{RoleOwner}},
}

// RequireRole returns middleware that checks the JWT role against allowed roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	roleSet := make(map[string]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := GetClaims(r)
			if err != nil {
				// No claims means dev mode (no secret set) — allow through
				next.ServeHTTP(w, r)
				return
			}
			if !roleSet[claims.Role] {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, ErrInsufficientRole.Error()), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CheckPermission checks if the given role is allowed to access method+path.
// Returns true if allowed. Owner always has access.
func CheckPermission(role, method, path string) bool {
	if role == RoleOwner {
		return true
	}

	// Normalize path: strip trailing slash for matching
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}

	// Check specific patterns first (longest match wins)
	for _, perm := range permissions {
		if matchRoute(perm.Pattern, path) && (perm.Method == "*" || perm.Method == method) {
			for _, r := range perm.Roles {
				if r == role {
					return true
				}
			}
			// Matched pattern but role not in list — check if there's a more specific match
			continue
		}
	}

	// Agent role: only specific endpoints
	if role == RoleAgent {
		// Agent can GET own genome and behavior, POST feedback
		if method == "GET" && (strings.Contains(path, "/genome") || strings.Contains(path, "/genome/behavior")) {
			return true
		}
		if method == "POST" && strings.Contains(path, "/feedback") {
			return true
		}
		return false
	}

	// Readonly: any GET on /api/
	if role == RoleReadonly {
		return method == "GET" && strings.HasPrefix(path, "/api/")
	}

	return false
}

// matchRoute checks if a path matches a route pattern (prefix-based with {id} wildcards).
func matchRoute(pattern, path string) bool {
	// Simple prefix matching with wildcard segments
	patParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(pathParts) < len(patParts) {
		// Allow prefix match if pattern ends with empty last segment
		if pattern == "/api/" && strings.HasPrefix(path, "/api") {
			return true
		}
		return false
	}

	for i, pp := range patParts {
		if strings.HasPrefix(pp, "{") && strings.HasSuffix(pp, "}") {
			continue // wildcard
		}
		if pp != pathParts[i] {
			return false
		}
	}
	return true
}
