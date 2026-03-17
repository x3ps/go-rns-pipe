package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// Config holds the runtime configuration for rns-tcp-iface.
type Config struct {
	// Mode is either "client" or "server".
	Mode string

	// Name is the interface name reported to RNS.
	Name string

	// ListenAddr is the address to listen on in server mode.
	ListenAddr string

	// PeerAddr is the address to connect to in client mode.
	PeerAddr string

	// MTU is the RNS packet MTU.
	MTU int

	// PipePath is unused but reserved for future subprocess spawning.
	PipePath string

	// ReconnectDelay is the base delay between client reconnection attempts.
	ReconnectDelay time.Duration

	// LogLevel controls log verbosity.
	LogLevel slog.Level
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	defaults := rnspipe.DefaultConfig()
	return Config{
		Name:           "TCPInterface",
		ListenAddr:     ":4242",
		MTU:            defaults.MTU,
		ReconnectDelay: 5 * time.Second,
		LogLevel:       slog.LevelInfo,
	}
}

// ParseConfig builds a Config by layering defaults, environment variables, and
// CLI flags (highest priority).
func ParseConfig() Config {
	cfg := DefaultConfig()

	// Environment variables (middle priority).
	if v := os.Getenv("RNS_TCP_MODE"); v != "" {
		cfg.Mode = v
	}
	if v := os.Getenv("RNS_TCP_NAME"); v != "" {
		cfg.Name = v
	}
	if v := os.Getenv("RNS_TCP_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("RNS_TCP_PEER_ADDR"); v != "" {
		cfg.PeerAddr = v
	}

	// CLI flags (highest priority).
	flag.StringVar(&cfg.Mode, "mode", cfg.Mode, "operating mode: client or server")
	flag.StringVar(&cfg.Name, "name", cfg.Name, "interface name reported to RNS")
	flag.StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "listen address for server mode")
	flag.StringVar(&cfg.PeerAddr, "peer-addr", cfg.PeerAddr, "peer address for client mode")
	flag.IntVar(&cfg.MTU, "mtu", cfg.MTU, "RNS packet MTU in bytes")
	flag.DurationVar(&cfg.ReconnectDelay, "reconnect-delay", cfg.ReconnectDelay, "base reconnect delay for client mode")

	var logLevel string
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: rns-tcp-iface [flags]\n\n")
		fmt.Fprintf(os.Stderr, "A TCP transport for Reticulum, bridging HDLC-over-pipe to HDLC-over-TCP.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  RNS_TCP_MODE          operating mode (client/server)\n")
		fmt.Fprintf(os.Stderr, "  RNS_TCP_NAME          interface name\n")
		fmt.Fprintf(os.Stderr, "  RNS_TCP_LISTEN_ADDR   listen address (server)\n")
		fmt.Fprintf(os.Stderr, "  RNS_TCP_PEER_ADDR     peer address (client)\n")
	}

	flag.Parse()

	switch logLevel {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	}

	return cfg
}

// Validate checks that the configuration is valid.
func (c Config) Validate() error {
	if c.Mode != "client" && c.Mode != "server" {
		return fmt.Errorf("--mode must be 'client' or 'server', got %q", c.Mode)
	}
	if c.Mode == "client" && c.PeerAddr == "" {
		return fmt.Errorf("--peer-addr is required in client mode")
	}
	return nil
}
