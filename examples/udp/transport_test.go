package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

func TestUDPTransportLifecycle(t *testing.T) {
	t.Parallel()

	// Use ephemeral port to avoid conflicts.
	cfg := Config{
		ListenAddr: "127.0.0.1:0",
		PeerAddr:   "127.0.0.1:1",
		Name:       "test-lifecycle",
		MTU:        500,
	}
	transport := NewTransport(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Inject io.Pipe so Start doesn't use real stdin/stdout.
	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()
	transport.pipeConfig = rnspipe.Config{
		Stdin:  stdinR,
		Stdout: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- transport.Start(ctx) }()

	// Give the transport time to start up.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("transport did not shut down within 3s after context cancel")
	}
}

func TestOpenUDPConn(t *testing.T) {
	t.Parallel()

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	conn, err := openUDPConn(addr)
	if err != nil {
		t.Fatalf("openUDPConn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
	}()

	if conn.LocalAddr() == nil {
		t.Fatal("expected non-nil local addr")
	}
}

func TestReadLoop(t *testing.T) {
	t.Parallel()

	// Build an rnspipe.Interface with in-memory pipes so it can start without
	// stdin/stdout affecting the test process.
	stdinR, _ := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	iface := rnspipe.New(rnspipe.Config{
		Stdin:  stdinR,
		Stdout: stdoutW,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	iface.OnSend(func([]byte) error { return nil })

	// Drain stdout so Receive() writes don't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdoutR.Read(buf); err != nil {
				return
			}
		}
	}()

	ifaceCtx, ifaceCancel := context.WithCancel(context.Background())
	defer ifaceCancel()
	go func() { _ = iface.Start(ifaceCtx) }()

	// Wait for the interface to come online.
	deadline := time.After(2 * time.Second)
	for !iface.IsOnline() {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for iface online")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Bind a real UDP socket for the read loop.
	conn, err := openUDPConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("openUDPConn: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
	}()

	transport := NewTransport(Config{MTU: 500}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Run readLoop until ctx is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan error, 1)
	go func() { loopDone <- transport.readLoop(ctx, conn, iface) }()

	// Send a datagram to the transport's listen address.
	payload := []byte("test-payload")
	sender, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("sender ListenUDP: %v", err)
	}
	defer func() {
		if err := sender.Close(); err != nil {
			t.Errorf("sender.Close: %v", err)
		}
	}()

	if _, err := sender.WriteTo(payload, conn.LocalAddr()); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	// Give the read loop time to process the datagram.
	time.Sleep(300 * time.Millisecond)

	cancel()

	select {
	case err := <-loopDone:
		if err != nil && err != context.Canceled {
			t.Errorf("readLoop returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not return after context cancel")
	}
}

// TestUDPTransportPipeClose verifies that when the pipe closes (stdin EOF),
// Start returns ErrPipeClosed instead of blocking forever in the socket loop.
func TestUDPTransportPipeClose(t *testing.T) {
	t.Parallel()

	stdinR, stdinW := io.Pipe()

	cfg := Config{
		ListenAddr: "127.0.0.1:0",
		PeerAddr:   "127.0.0.1:1",
		Name:       "test-pipe-close",
		MTU:        500,
	}
	transport := NewTransport(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	transport.pipeConfig = rnspipe.Config{
		Stdin:  stdinR,
		Stdout: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- transport.Start(ctx) }()

	// Give the transport time to start up.
	time.Sleep(100 * time.Millisecond)

	// Close the write end of stdin — pipe sees clean EOF → ErrPipeClosed.
	_ = stdinW.Close()

	select {
	case err := <-done:
		if !errors.Is(err, rnspipe.ErrPipeClosed) {
			t.Fatalf("expected ErrPipeClosed, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after pipe close")
	}
}
