package cloudsync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTursoClient_Execute(t *testing.T) {
	// Mock Turso server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/pipeline" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or invalid auth token")
		}

		// Return success response
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type:         "ok",
					RowsAffected: 1,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	ctx := context.Background()

	err := client.Execute(ctx, "INSERT INTO test VALUES (?)", "value1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestTursoClient_Query(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"id", "name"},
						Rows: [][]interface{}{
							{"1", "test"},
							{"2", "test2"},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	ctx := context.Background()

	result, err := client.Query(ctx, "SELECT * FROM test")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}

	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
}

func TestTursoClient_BatchExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PipelineRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Requests) != 3 {
			t.Errorf("expected 3 requests, got %d", len(req.Requests))
		}

		resp := PipelineResponse{
			Results: []BatchResult{
				{Type: "ok", RowsAffected: 1},
				{Type: "ok", RowsAffected: 1},
				{Type: "ok", RowsAffected: 1},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	ctx := context.Background()

	statements := []Statement{
		{SQL: "INSERT INTO test VALUES (?)", Args: []interface{}{"val1"}},
		{SQL: "INSERT INTO test VALUES (?)", Args: []interface{}{"val2"}},
		{SQL: "INSERT INTO test VALUES (?)", Args: []interface{}{"val3"}},
	}

	err := client.BatchExecute(ctx, statements)
	if err != nil {
		t.Fatalf("BatchExecute failed: %v", err)
	}
}

func TestTursoClient_RetryOnFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			// Fail first attempt
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on second attempt
		resp := PipelineResponse{
			Results: []BatchResult{{Type: "ok"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	ctx := context.Background()

	err := client.Execute(ctx, "INSERT INTO test VALUES (?)", "value")
	if err != nil {
		t.Fatalf("Execute should succeed after retry: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestTursoClient_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "error",
					Error: &PipelineError{
						Message: "table not found",
						Code:    "NOT_FOUND",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	ctx := context.Background()

	err := client.Execute(ctx, "SELECT * FROM nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "NOT_FOUND: table not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTursoClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		resp := PipelineResponse{
			Results: []BatchResult{{Type: "ok"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	client.httpClient.Timeout = 10 * time.Millisecond

	ctx := context.Background()
	err := client.Execute(ctx, "INSERT INTO test VALUES (?)", "value")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
