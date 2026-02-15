package cloudsync

import (
	"encoding/json"
	"testing"
)

func TestToTursoValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected map[string]interface{}
	}{
		{
			name:  "nil value",
			input: nil,
			expected: map[string]interface{}{
				"type": "null",
			},
		},
		{
			name:  "string value",
			input: "dell-xps-agent",
			expected: map[string]interface{}{
				"type":  "text",
				"value": "dell-xps-agent",
			},
		},
		{
			name:  "uuid string",
			input: "5e87d738-f7d8-49ee-8dfb-91e45a963006",
			expected: map[string]interface{}{
				"type":  "text",
				"value": "5e87d738-f7d8-49ee-8dfb-91e45a963006",
			},
		},
		{
			name:  "integer value",
			input: 42,
			expected: map[string]interface{}{
				"type":  "integer",
				"value": "42",
			},
		},
		{
			name:  "int64 value",
			input: int64(1234567890),
			expected: map[string]interface{}{
				"type":  "integer",
				"value": "1234567890",
			},
		},
		{
			name:  "float value",
			input: 3.14,
			expected: map[string]interface{}{
				"type":  "float",
				"value": 3.14,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTursoValue(tt.input)
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map[string]interface{}, got %T", result)
			}

			if resultMap["type"] != tt.expected["type"] {
				t.Errorf("expected type %v, got %v", tt.expected["type"], resultMap["type"])
			}

			if tt.expected["value"] != nil && resultMap["value"] != tt.expected["value"] {
				t.Errorf("expected value %v, got %v", tt.expected["value"], resultMap["value"])
			}
		})
	}
}

func TestConvertArgs(t *testing.T) {
	args := []interface{}{
		"dell-xps-agent",
		"5e87d738-f7d8-49ee-8dfb-91e45a963006",
		"evoclaw-device",
		"orchestrator",
		int64(1234567890),
	}

	converted := convertArgs(args)

	if len(converted) != len(args) {
		t.Fatalf("expected %d args, got %d", len(args), len(converted))
	}

	// Check first arg (device_id)
	arg0, ok := converted[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for arg 0, got %T", converted[0])
	}
	if arg0["type"] != "text" {
		t.Errorf("expected type=text for arg 0, got %v", arg0["type"])
	}
	if arg0["value"] != "dell-xps-agent" {
		t.Errorf("expected value=dell-xps-agent for arg 0, got %v", arg0["value"])
	}

	// Verify it can be marshaled to JSON without errors
	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("failed to marshal converted args: %v", err)
	}

	t.Logf("Converted args JSON: %s", string(jsonBytes))
}

func TestStatementWithConvertedArgs(t *testing.T) {
	stmt := Statement{
		SQL: "INSERT INTO devices (device_id, agent_id) VALUES (?, ?)",
		Args: []interface{}{
			"dell-xps-agent",
			"5e87d738-f7d8-49ee-8dfb-91e45a963006",
		},
	}

	converted := convertArgs(stmt.Args)

	// Verify JSON structure matches Turso spec
	jsonBytes, err := json.Marshal(map[string]interface{}{
		"sql":  stmt.SQL,
		"args": converted,
	})
	if err != nil {
		t.Fatalf("failed to marshal statement: %v", err)
	}

	t.Logf("Statement JSON: %s", string(jsonBytes))

	// Verify the structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to parse back: %v", err)
	}

	args, ok := parsed["args"].([]interface{})
	if !ok {
		t.Fatalf("expected args array, got %T", parsed["args"])
	}

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}

	// Check first arg structure
	arg0, ok := args[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for arg 0, got %T", args[0])
	}

	if arg0["type"] != "text" || arg0["value"] != "dell-xps-agent" {
		t.Errorf("arg 0 incorrect: %+v", arg0)
	}
}
