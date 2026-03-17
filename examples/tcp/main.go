// rns-tcp-iface is a TCP transport for Reticulum, equivalent to Python RNS's
// TCPClientInterface and TCPServerInterface.
//
// It bridges HDLC-framed traffic between a pipe to rnsd (stdin/stdout) and
// one or more TCP connections, using the same HDLC framing on both sides:
//
//	rnsd ←[HDLC/pipe]→ rns-tcp-iface ←[HDLC/TCP]→ remote peer(s)
//
// Protocol notes (from TCPInterface.py):
//   - Framing: HDLC (FLAG=0x7E, ESC=0x7D, ESC_MASK=0x20) — same as PipeInterface
//   - No handshake: connection is immediate, raw HDLC on connect
//   - TCP_NODELAY: enabled for low-latency packet delivery
//   - Client: reconnects with exponential backoff on disconnect
//   - Server: accepts multiple clients; broadcasts pipe→TCP to all
//
// This differs from PipeInterface in that the TCP side requires its own HDLC
// encode/decode (PipeInterface only handles the pipe side). We reuse the
// library's Encoder and Decoder for both.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
	cfg := ParseConfig()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})).With("name", cfg.Name)

	// Create pipe interface connected to rnsd via stdin/stdout.
	iface := rnspipe.New(rnspipe.Config{
		Name:   cfg.Name,
		MTU:    cfg.MTU,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
	})

	iface.OnStatus(func(online bool) {
		if online {
			logger.Info("pipe interface online")
		} else {
			logger.Warn("pipe interface offline")
		}
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run pipe interface and TCP transport concurrently.
	// First error triggers shutdown of both.
	errc := make(chan error, 2)

	go func() { errc <- iface.Start(ctx) }()

	go func() {
		switch cfg.Mode {
		case "client":
			errc <- runClient(ctx, cfg, iface, logger)
		case "server":
			errc <- runServer(ctx, cfg, iface, logger)
		}
	}()

	// Wait for first error.
	err := <-errc
	cancel()
	// Wait for second goroutine to finish.
	<-errc

	if err != nil && ctx.Err() == nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
