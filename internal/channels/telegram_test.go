package channels

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

func TestNewTelegram(t *testing.T) {
	tg := NewTelegram("test-token", testLogger())
	
	if tg == nil {
		t.Fatal("expected non-nil telegram client")
	}
	
	if tg.Name() != "telegram" {
		t.Errorf("expected name telegram, got %s", tg.Name())
	}
}

func TestTelegramSendWithoutStart(t *testing.T) {
	tg := NewTelegram("test-token", testLogger())
	
	// Sending without Start should work (it will just fail to connect to Telegram)
	msg := orchestrator.Response{
		Content: "Hello, world!",
		To:      "12345",
		Channel: "telegram",
	}
	
	// This will return an error because we're not actually connected to Telegram,
	// but we're testing that the method doesn't panic and handles the case
	err := tg.Send(context.Background(), msg)
	// We expect an error because the token is invalid and we can't reach Telegram API
	if err == nil {
		t.Log("Note: Send succeeded (possibly connected to real Telegram API)")
	}
}

func TestNewMQTT(t *testing.T) {
	mqtt := NewMQTT("localhost", 1883, "user", "pass", testLogger())
	
	if mqtt == nil {
		t.Fatal("expected non-nil MQTT channel")
	}
	
	if mqtt.Name() != "mqtt" {
		t.Errorf("expected name mqtt, got %s", mqtt.Name())
	}
	
	if mqtt.broker != "localhost" {
		t.Errorf("expected broker localhost, got %s", mqtt.broker)
	}
	
	if mqtt.port != 1883 {
		t.Errorf("expected port 1883, got %d", mqtt.port)
	}
}
