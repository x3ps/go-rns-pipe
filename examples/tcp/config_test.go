package main

import (
	"testing"
	"time"
)

func TestValidateReconnectDelayZero(t *testing.T) {
	cfg := Config{Mode: "client", PeerAddr: "localhost:4242", ReconnectDelay: 0}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero ReconnectDelay in client mode, got nil")
	}
}

func TestValidateReconnectDelayNegative(t *testing.T) {
	cfg := Config{Mode: "client", PeerAddr: "localhost:4242", ReconnectDelay: -1 * time.Second}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative ReconnectDelay in client mode, got nil")
	}
}

func TestValidateReconnectDelayPositive(t *testing.T) {
	cfg := Config{Mode: "client", PeerAddr: "localhost:4242", ReconnectDelay: 5 * time.Second}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error for positive ReconnectDelay: %v", err)
	}
}

func TestValidateReconnectDelayServerMode(t *testing.T) {
	cfg := Config{Mode: "server", ListenAddr: ":4242", ReconnectDelay: 0}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error for zero ReconnectDelay in server mode: %v", err)
	}
}
