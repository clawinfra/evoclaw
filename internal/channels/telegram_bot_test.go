package channels

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input   string
		command string
		args    string
	}{
		{"/status", "status", ""},
		{"/agents", "agents", ""},
		{"/agent pi1-edge", "agent", "pi1-edge"},
		{"/ask What is the temperature?", "ask", "What is the temperature?"},
		{"/skills pi1-edge", "skills", "pi1-edge"},
		{"/help", "help", ""},
		{"/start", "start", ""},
		{"hello", "", "hello"},
		{"", "", ""},
		{"/AGENT PI1-EDGE", "agent", "PI1-EDGE"},
		{"/ask  multiple   spaces", "ask", "multiple   spaces"},
		{"/status@mybot", "status", ""},
		{"/agent@mybot pi1-edge", "agent", "pi1-edge"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, args := ParseCommand(tt.input)
			if cmd != tt.command {
				t.Errorf("ParseCommand(%q) command = %q, want %q", tt.input, cmd, tt.command)
			}
			if args != tt.args {
				t.Errorf("ParseCommand(%q) args = %q, want %q", tt.input, args, tt.args)
			}
		})
	}
}

func TestTelegramBot_IsUserAllowed(t *testing.T) {
	// Empty list = allow all
	bot := &TelegramBot{
		allowedUsers: []int64{},
	}
	if !bot.isUserAllowed("12345") {
		t.Error("expected all users allowed when list is empty")
	}

	// With specific users
	bot.allowedUsers = []int64{100, 200, 300}
	if !bot.isUserAllowed("100") {
		t.Error("expected user 100 to be allowed")
	}
	if !bot.isUserAllowed("200") {
		t.Error("expected user 200 to be allowed")
	}
	if bot.isUserAllowed("999") {
		t.Error("expected user 999 to be denied")
	}
	if bot.isUserAllowed("invalid") {
		t.Error("expected invalid user ID to be denied")
	}
}

func TestTelegramBot_GetAgentForUser(t *testing.T) {
	bot := &TelegramBot{
		defaultAgent: "default-agent",
		userAgents:   make(map[string]string),
	}

	// No override = default
	if agent := bot.getAgentForUser("user1"); agent != "default-agent" {
		t.Errorf("expected default-agent, got %s", agent)
	}

	// With override
	bot.userAgents["user1"] = "custom-agent"
	if agent := bot.getAgentForUser("user1"); agent != "custom-agent" {
		t.Errorf("expected custom-agent, got %s", agent)
	}

	// Other user still gets default
	if agent := bot.getAgentForUser("user2"); agent != "default-agent" {
		t.Errorf("expected default-agent, got %s", agent)
	}
}
