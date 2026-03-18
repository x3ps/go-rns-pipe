// rns-udp-iface is a UDP transport for Reticulum, equivalent to Python RNS's
// UDPInterface.
//
// It bridges HDLC-framed traffic between a pipe to rnsd (stdin/stdout) and
// the network via raw UDP datagrams:
//
//	rnsd ←[HDLC/pipe]→ rns-udp-iface ←[raw datagram]→ remote peers
//
// Protocol notes (from UDPInterface.py):
//   - No HDLC framing on the UDP side — each datagram IS a raw RNS packet.
//     Datagram boundaries delimit packets naturally (unlike TCP).
//   - SO_BROADCAST is always enabled, supporting both unicast and broadcast peers.
//   - No source-IP filtering: packets are accepted from all senders.
//
// This is the simplest official example: stateless, no reconnect logic, no
// client/server split. Read transport.go once to understand the full library API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
	cfg := ParseConfig()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})).With("name", cfg.Name)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	t := NewTransport(cfg, logger)
	if err := t.Start(ctx); err != nil && ctx.Err() == nil {
		if errors.Is(err, rnspipe.ErrPipeClosed) {
			logger.Info("pipe closed by rnsd, exiting for respawn")
			os.Exit(0)
		}
		logger.Error("transport error", "err", err)
		os.Exit(1)
	}
}
