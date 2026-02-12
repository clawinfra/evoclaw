package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"
)

// Client is a Turso HTTP API client with zero CGO dependencies
type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Turso client
func NewClient(databaseURL, authToken string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	// Convert libsql:// to https:// for HTTP API
	baseURL := databaseURL
	if len(baseURL) >= 9 && baseURL[:9] == "libsql://" {
		baseURL = "https://" + baseURL[9:]
	}

	return &Client{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// PipelineRequest represents a batch of operations
type PipelineRequest struct {
	Requests []BatchRequest `json:"requests"`
}

// BatchRequest is a single operation in a batch
type BatchRequest struct {
	Type      string    `json:"type"` // "execute" or "close"
	Statement Statement `json:"stmt,omitempty"`
}

// Statement is a SQL statement with parameters
type Statement struct {
	SQL  string        `json:"sql"`
	Args []interface{} `json:"args,omitempty"`
}

// PipelineResponse is the response from a batch operation
type PipelineResponse struct {
	Results []BatchResult `json:"results"`
}

// BatchResult is the result of a single operation
type BatchResult struct {
	Type         string          `json:"type"` // "ok" or "error"
	Response     *QueryResponse  `json:"response,omitempty"`
	RowsAffected int64           `json:"rows_affected,omitempty"`
	LastInsertID *string         `json:"last_insert_rowid,omitempty"`
	Error        *PipelineError  `json:"error,omitempty"`
}

// QueryResponse contains query results
type QueryResponse struct {
	Columns []string        `json:"cols"`
	Rows    [][]interface{} `json:"rows"`
}

// PipelineError represents an error from Turso
type PipelineError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (e *PipelineError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

// toTursoValue converts a Go value to Turso's internally tagged enum Value format
// According to Hrana spec: https://github.com/tursodatabase/libsql/blob/main/docs/HRANA_1_SPEC.md
func toTursoValue(v interface{}) interface{} {
	if v == nil {
		return map[string]interface{}{"type": "null"}
	}

	switch val := v.(type) {
	case string:
		return map[string]interface{}{
			"type":  "text",
			"value": val,
		}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return map[string]interface{}{
			"type":  "integer",
			"value": fmt.Sprintf("%d", val),
		}
	case float32, float64:
		return map[string]interface{}{
			"type":  "float",
			"value": val,
		}
	case []byte:
		// Base64 encode binary data
		return map[string]interface{}{
			"type":   "blob",
			"base64": string(val), // Assuming already base64 encoded
		}
	default:
		// Fallback: treat as text
		return map[string]interface{}{
			"type":  "text",
			"value": fmt.Sprintf("%v", val),
		}
	}
}

// convertArgs converts a slice of Go values to Turso's Value format
func convertArgs(args []interface{}) []interface{} {
	if args == nil {
		return nil
	}
	converted := make([]interface{}, len(args))
	for i, arg := range args {
		converted[i] = toTursoValue(arg)
	}
	return converted
}

// executePipeline executes a batch of operations with retry and exponential backoff
func (c *Client) executePipeline(ctx context.Context, req PipelineRequest) (*PipelineResponse, error) {
	var lastErr error
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			c.logger.Debug("retrying turso request",
				"attempt", attempt+1,
				"delay", delay)
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.doExecutePipeline(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		c.logger.Warn("turso request failed",
			"attempt", attempt+1,
			"error", err)
	}

	return nil, fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

// doExecutePipeline performs the actual HTTP request
func (c *Client) doExecutePipeline(ctx context.Context, req PipelineRequest) (*PipelineResponse, error) {
	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build HTTP request
	url := c.baseURL + "/v2/pipeline"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)

	// Execute request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check HTTP status
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Parse response
	var resp PipelineResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}

// Execute runs a single SQL statement
func (c *Client) Execute(ctx context.Context, sql string, args ...interface{}) error {
	req := PipelineRequest{
		Requests: []BatchRequest{
			{
				Type: "execute",
				Statement: Statement{
					SQL:  sql,
					Args: convertArgs(args),
				},
			},
		},
	}

	resp, err := c.executePipeline(ctx, req)
	if err != nil {
		return err
	}

	if len(resp.Results) > 0 && resp.Results[0].Type == "error" {
		return resp.Results[0].Error
	}

	return nil
}

// Query runs a SQL query and returns rows
func (c *Client) Query(ctx context.Context, sql string, args ...interface{}) (*QueryResponse, error) {
	req := PipelineRequest{
		Requests: []BatchRequest{
			{
				Type: "execute",
				Statement: Statement{
					SQL:  sql,
					Args: convertArgs(args),
				},
			},
		},
	}

	resp, err := c.executePipeline(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return &QueryResponse{}, nil
	}

	result := resp.Results[0]
	if result.Type == "error" {
		return nil, result.Error
	}

	if result.Response != nil {
		return result.Response, nil
	}

	return &QueryResponse{}, nil
}

// QueryOne returns a single row
func (c *Client) QueryOne(ctx context.Context, sql string, args ...interface{}) ([]interface{}, error) {
	resp, err := c.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	if len(resp.Rows) == 0 {
		return nil, fmt.Errorf("no rows found")
	}

	return resp.Rows[0], nil
}

// BatchExecute runs multiple statements in a transaction
func (c *Client) BatchExecute(ctx context.Context, statements []Statement) error {
	requests := make([]BatchRequest, len(statements))
	for i, stmt := range statements {
		// Convert args to Turso format
		convertedStmt := Statement{
			SQL:  stmt.SQL,
			Args: convertArgs(stmt.Args),
		}
		requests[i] = BatchRequest{
			Type:      "execute",
			Statement: convertedStmt,
		}
	}

	req := PipelineRequest{Requests: requests}
	resp, err := c.executePipeline(ctx, req)
	if err != nil {
		return err
	}

	// Check for any errors in batch
	for i, result := range resp.Results {
		if result.Type == "error" {
			return fmt.Errorf("statement %d failed: %w", i, result.Error)
		}
	}

	return nil
}

// currentTimestamp returns Unix timestamp in seconds
func currentTimestamp() int64 {
	return time.Now().Unix()
}
