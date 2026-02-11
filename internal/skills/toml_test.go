package skills

import (
	"testing"
)

func TestParseToolsTOML(t *testing.T) {
	data := `
# Tool definitions
[tools.search]
command = "search"
description = "Search the web"
timeout_secs = 30
args = ["--format", "json"]
env = ["API_KEY=test"]

[tools.calculator]
command = "calc"
description = "Simple calculator"
timeout_secs = 5
`

	tools, err := ParseToolsTOML([]byte(data))
	if err != nil {
		t.Fatalf("ParseToolsTOML() error: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	search := tools["search"]
	if search == nil {
		t.Fatal("search tool not found")
	}
	if search.Command != "search" {
		t.Errorf("command = %q, want search", search.Command)
	}
	if search.Description != "Search the web" {
		t.Errorf("description = %q", search.Description)
	}
	if search.TimeoutSecs != 30 {
		t.Errorf("timeout = %d, want 30", search.TimeoutSecs)
	}
	if len(search.Args) != 2 {
		t.Errorf("args len = %d, want 2", len(search.Args))
	}
	if len(search.Env) != 1 {
		t.Errorf("env len = %d, want 1", len(search.Env))
	}
}

func TestParseToolsTOMLEmpty(t *testing.T) {
	tools, err := ParseToolsTOML([]byte(""))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestParseToolsTOMLComments(t *testing.T) {
	data := `
# Comment
[tools.test]
# Another comment
command = "test"
`
	tools, err := ParseToolsTOML([]byte(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if tools["test"] == nil || tools["test"].Command != "test" {
		t.Error("failed to parse with comments")
	}
}

func TestParseToolsTOMLNonToolSection(t *testing.T) {
	data := `
[other]
key = "value"

[tools.test]
command = "test"
`
	tools, err := ParseToolsTOML([]byte(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

func TestParseToolsTOMLInvalidArray(t *testing.T) {
	data := `
[tools.test]
args = not_an_array
`
	_, err := ParseToolsTOML([]byte(data))
	if err == nil {
		t.Error("expected error for invalid array")
	}
}

func TestUnquote(t *testing.T) {
	tests := []struct{ in, want string }{
		{`"hello"`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`"a"`, "a"},
	}
	for _, tt := range tests {
		if got := unquote(tt.in); got != tt.want {
			t.Errorf("unquote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseStringArray(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{`["a", "b"]`, 2, false},
		{`[]`, 0, false},
		{`["single"]`, 1, false},
		{`not-array`, 0, true},
	}
	for _, tt := range tests {
		got, err := parseStringArray(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseStringArray(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if !tt.wantErr && len(got) != tt.wantLen {
			t.Errorf("parseStringArray(%q) len = %d, want %d", tt.input, len(got), tt.wantLen)
		}
	}
}
