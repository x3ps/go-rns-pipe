package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// Config holds the runtime configuration for rns-udp-iface.
type Config struct {
	// ListenAddr is the UDP address to listen on for incoming datagrams.
	ListenAddr string

	// PeerAddr is the UDP address to send outgoing packets to.
	// Use a broadcast address (e.g. 255.255.255.255:4242) for broadcast mode.
	PeerAddr string

	// Name is the interface name reported to RNS.
	Name string

	// MTU is the RNS packet MTU.
	MTU int

	// LogLevel controls log verbosity.
	LogLevel slog.Level
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	defaults := rnspipe.DefaultConfig()
	return Config{
		ListenAddr: "0.0.0.0:4242",
		PeerAddr:   "255.255.255.255:4242",
		Name:       "UDPInterface",
		MTU:        defaults.MTU,
		LogLevel:   slog.LevelInfo,
	}
}

// ParseConfig builds a Config by layering defaults, environment variables, and
// CLI flags (highest priority).
func ParseConfig() Config {
	cfg := DefaultConfig()
	logLevel := "info"

	// Environment variables (middle priority).
	if v := os.Getenv("RNS_UDP_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("RNS_UDP_PEER_ADDR"); v != "" {
		cfg.PeerAddr = v
	}
	if v := os.Getenv("RNS_UDP_NAME"); v != "" {
		cfg.Name = v
	}
	if v := os.Getenv("RNS_UDP_MTU"); v != "" {
		if mtu, err := strconv.Atoi(v); err == nil {
			cfg.MTU = mtu
		}
	}
	if v := os.Getenv("RNS_UDP_LOG_LEVEL"); v != "" {
		logLevel = v
	}

	// CLI flags (highest priority).
	fs := flag.NewFlagSet("rns-udp-iface", flag.ContinueOnError)
	fs.StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "UDP address to listen on")
	fs.StringVar(&cfg.PeerAddr, "peer-addr", cfg.PeerAddr, "UDP address to send packets to (broadcast or unicast)")
	fs.StringVar(&cfg.Name, "name", cfg.Name, "interface name reported to RNS")
	fs.IntVar(&cfg.MTU, "mtu", cfg.MTU, "RNS packet MTU in bytes")

	fs.StringVar(&logLevel, "log-level", logLevel, "log level: debug, info, warn, error")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: rns-udp-iface [flags]\n\n")
		fmt.Fprintf(os.Stderr, "A UDP transport for Reticulum, bridging HDLC-over-pipe to raw UDP datagrams.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  RNS_UDP_LISTEN_ADDR   UDP address to listen on (default: 0.0.0.0:4242)\n")
		fmt.Fprintf(os.Stderr, "  RNS_UDP_PEER_ADDR     UDP address to send to (default: 255.255.255.255:4242)\n")
		fmt.Fprintf(os.Stderr, "  RNS_UDP_NAME          interface name (default: UDPInterface)\n")
		fmt.Fprintf(os.Stderr, "  RNS_UDP_MTU           packet MTU in bytes\n")
		fmt.Fprintf(os.Stderr, "  RNS_UDP_LOG_LEVEL     log level: debug, info, warn, error\n")
	}

	_ = fs.Parse(os.Args[1:])

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
