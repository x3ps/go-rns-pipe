package main

import (
	"log/slog"
	"os"
	"testing"
)

func TestParseConfigReadsEnvOverrides(t *testing.T) {
	t.Setenv("RNS_UDP_LISTEN_ADDR", "127.0.0.1:5001")
	t.Setenv("RNS_UDP_PEER_ADDR", "127.0.0.1:5002")
	t.Setenv("RNS_UDP_NAME", "udp-test")
	t.Setenv("RNS_UDP_MTU", "777")
	t.Setenv("RNS_UDP_LOG_LEVEL", "debug")

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"rns-udp-iface"}

	cfg := ParseConfig()

	if cfg.ListenAddr != "127.0.0.1:5001" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:5001")
	}
	if cfg.PeerAddr != "127.0.0.1:5002" {
		t.Fatalf("PeerAddr = %q, want %q", cfg.PeerAddr, "127.0.0.1:5002")
	}
	if cfg.Name != "udp-test" {
		t.Fatalf("Name = %q, want %q", cfg.Name, "udp-test")
	}
	if cfg.MTU != 777 {
		t.Fatalf("MTU = %d, want %d", cfg.MTU, 777)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelDebug)
	}
}

func TestParseConfigKeepsDefaultMTUOnInvalidEnv(t *testing.T) {
	t.Setenv("RNS_UDP_MTU", "not-a-number")

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"rns-udp-iface"}

	cfg := ParseConfig()

	if cfg.MTU != DefaultConfig().MTU {
		t.Fatalf("MTU = %d, want default %d", cfg.MTU, DefaultConfig().MTU)
	}
}
