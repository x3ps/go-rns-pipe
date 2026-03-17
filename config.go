package rnspipe

import (
	"io"
	"log/slog"
	"time"
)

// Config holds the configuration for an RNS PipeInterface.
type Config struct {
	// Name is the interface name as it will appear in RNS logs.
	Name string

	// MTU in bytes. Defaults to 500 — the standard RNS on-wire MTU.
	// See: Interface.py — RNS uses 500-byte physical MTU by default.
	MTU int

	// HWMTU is the hardware-level maximum transfer unit used for HDLC buffer
	// sizing. Defaults to 1064, matching PipeInterface.py.
	// See: PipeInterface.py#L72 — self.HWMTU = 1064
	HWMTU int

	// Bitrate in bits/s. Defaults to 1000000 (1 Mbps), matching
	// PipeInterface.BITRATE_GUESS.
	// See: PipeInterface.py#L48 — BITRATE_GUESS = 1*1000*1000
	Bitrate int

	// ReconnectDelay is the base delay before attempting to reconnect after
	// a pipe failure. Defaults to 5s, matching PipeInterface respawn_delay.
	// See: PipeInterface.py#L67 — respawn_delay default = 5
	ReconnectDelay time.Duration

	// MaxReconnectAttempts is the maximum number of reconnection attempts after
	// the initial connection fails. 0 means infinite retries (default).
	// Example: MaxReconnectAttempts=1 allows one retry after the first failure.
	MaxReconnectAttempts int

	// LogLevel controls the verbosity of log output.
	LogLevel slog.Level

	// Logger is a custom structured logger. If nil, a default logger is created.
	Logger *slog.Logger

	// Stdin is the reader from which HDLC-framed packets are read (packets
	// from rnsd). Defaults to os.Stdin.
	//
	// Stdin should implement io.Closer so that context cancellation can
	// unblock the internal io.Copy goroutine. If Stdin does not implement
	// io.Closer, cancelling the context will return from readLoop but the
	// goroutine will remain blocked on the reader until the process exits.
	// os.Stdin is deliberately excluded from this close path (see readLoop).
	Stdin io.Reader

	// Stdout is the writer to which HDLC-framed packets are written (packets
	// to rnsd). Defaults to os.Stdout.
	Stdout io.Writer

	// ReceiveBufferSize is the capacity of the internal packet channel.
	// Defaults to 64.
	ReceiveBufferSize int
}

// DefaultConfig returns a Config with sensible defaults matching the Python
// RNS PipeInterface implementation.
func DefaultConfig() Config {
	return Config{
		Name:              "PipeInterface",
		MTU:               500,  // See: understanding.html — physical layer MTU of 500 bytes
		HWMTU:             1064, // See: PipeInterface.py#L72 — self.HWMTU = 1064
		Bitrate:           1_000_000,
		ReconnectDelay:    5 * time.Second,
		ReceiveBufferSize: 64,
	}
}
