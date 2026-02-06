package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloud"
)

func newMockAPIServer() *httptest.Server {
	mux := http.NewServeMux()
	now := time.Now()

	mux.HandleFunc("POST /api/cloud/spawn", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(cloud.Sandbox{
			SandboxID:  "sb-cli-test",
			AgentID:    "agent-cli",
			TemplateID: "evoclaw-agent",
			State:      "running",
			StartedAt:  now,
			EndsAt:     now.Add(5 * time.Minute),
		})
	})

	mux.HandleFunc("GET /api/cloud", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]cloud.Sandbox{
			{
				SandboxID:  "sb-1",
				AgentID:    "agent-1",
				TemplateID: "evoclaw-agent",
				State:      "running",
				StartedAt:  now.Add(-5 * time.Minute),
				EndsAt:     now.Add(5 * time.Minute),
			},
			{
				SandboxID:  "sb-2",
				AgentID:    "agent-2",
				TemplateID: "evoclaw-agent",
				State:      "running",
				StartedAt:  now.Add(-2 * time.Minute),
				EndsAt:     now.Add(8 * time.Minute),
			},
		})
	})

	mux.HandleFunc("DELETE /api/cloud/sb-cli-test", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"message": "killed"})
	})

	mux.HandleFunc("DELETE /api/cloud/sb-nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	mux.HandleFunc("GET /api/cloud/sb-cli-test", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cloud.Status{
			SandboxID: "sb-cli-test",
			AgentID:   "agent-cli",
			State:     "running",
			Healthy:   true,
			UptimeSec: 300,
			EndsAt:    now.Add(5 * time.Minute),
		})
	})

	mux.HandleFunc("GET /api/cloud/costs", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cloud.CostSnapshot{
			TotalSandboxes:   5,
			ActiveSandboxes:  2,
			TotalUptimeSec:   3600,
			EstimatedCostUSD: 0.36,
			BudgetUSD:        50.0,
			BudgetRemaining:  49.64,
		})
	})

	return httptest.NewServer(mux)
}

func TestNewCloudCLI(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	if cli == nil {
		t.Fatal("expected non-nil CLI")
	}
	if cli.apiURL != "http://localhost:8420" {
		t.Errorf("expected trimmed URL, got '%s'", cli.apiURL)
	}
}

func TestNewCloudCLI_TrailingSlash(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420/")
	if cli.apiURL != "http://localhost:8420" {
		t.Errorf("expected trimmed URL, got '%s'", cli.apiURL)
	}
}

func TestRunNoArgs(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	code := cli.Run([]string{})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunHelp(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	for _, arg := range []string{"help", "--help", "-h"} {
		code := cli.Run([]string{arg})
		if code != 0 {
			t.Errorf("expected exit code 0 for '%s', got %d", arg, code)
		}
	}
}

func TestRunUnknown(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	code := cli.Run([]string{"nonexistent"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunSpawn(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn", "--template", "evoclaw-agent", "--id", "test-agent"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunSpawn_Default(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunSpawn_WithGenome(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn", "--genome", `{"type":"momentum"}`})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunSpawn_BadConfig(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn", "--config", "/nonexistent/file.toml"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunSpawn_WithConfigFile(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	tmpFile := t.TempDir() + "/test-agent.toml"
	os.WriteFile(tmpFile, []byte("[agent]\nid = \"test\"\n"), 0644)

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn", "--config", tmpFile})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunSpawn_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"spawn"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunSpawn_ConnectionError(t *testing.T) {
	cli := NewCloudCLI("http://localhost:99999")
	code := cli.Run([]string{"spawn"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunList(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"list"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunList_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]cloud.Sandbox{})
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"list"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunList_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"list"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunList_ConnectionError(t *testing.T) {
	cli := NewCloudCLI("http://localhost:99999")
	code := cli.Run([]string{"list"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunKill(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"kill", "sb-cli-test"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunKill_NoArgs(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	code := cli.Run([]string{"kill"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunKill_NotFound(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"kill", "sb-nonexistent"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunKill_ConnectionError(t *testing.T) {
	cli := NewCloudCLI("http://localhost:99999")
	code := cli.Run([]string{"kill", "sb-1"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunLogs(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"logs", "sb-cli-test"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunLogs_NoArgs(t *testing.T) {
	cli := NewCloudCLI("http://localhost:8420")
	code := cli.Run([]string{"logs"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunLogs_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"logs", "sb-nonexistent"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunLogs_ConnectionError(t *testing.T) {
	cli := NewCloudCLI("http://localhost:99999")
	code := cli.Run([]string{"logs", "sb-1"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunCosts(t *testing.T) {
	server := newMockAPIServer()
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"costs"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunCosts_LowBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(cloud.CostSnapshot{
			TotalSandboxes:   10,
			ActiveSandboxes:  5,
			TotalUptimeSec:   86400,
			EstimatedCostUSD: 48.0,
			BudgetUSD:        50.0,
			BudgetRemaining:  2.0,
		})
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"costs"})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunCosts_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cli := NewCloudCLIWithClient(server.URL, server.Client())
	code := cli.Run([]string{"costs"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRunCosts_ConnectionError(t *testing.T) {
	cli := NewCloudCLI("http://localhost:99999")
	code := cli.Run([]string{"costs"})
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{5*time.Minute + 30*time.Second, "5m 30s"},
		{2*time.Hour + 15*time.Minute + 45*time.Second, "2h 15m 45s"},
		{24 * time.Hour, "24h 0m 0s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.d)
		if result != tt.expected {
			t.Errorf("formatDuration(%v): expected '%s', got '%s'", tt.d, tt.expected, result)
		}
	}
}
