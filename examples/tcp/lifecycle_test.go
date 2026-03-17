package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestIface(stdin io.Reader, stdout io.Writer) *rnspipe.Interface {
	return rnspipe.New(rnspipe.Config{
		Stdin:  stdin,
		Stdout: stdout,
	})
}

// TestClientConnectDisconnect starts runClient against a local listener,
// accepts the connection, then cancels ctx and verifies clean shutdown.
func TestClientConnectDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	iface := newTestIface(stdinR, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	ifaceDone := make(chan error, 1)
	go func() { ifaceDone <- iface.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	cfg := Config{
		Mode:           "client",
		PeerAddr:       ln.Addr().String(),
		MTU:            500,
		ReconnectDelay: 100 * time.Millisecond,
	}

	clientDone := make(chan error, 1)
	go func() { clientDone <- runClient(ctx, cfg, iface, discardLogger()) }()

	// Accept the incoming connection from runClient.
	tcpLn := ln.(*net.TCPListener)
	if err := tcpLn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	conn, err := ln.Accept()
	if err != nil {
		cancel()
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	cancel()
	_ = stdinW.Close()

	select {
	case <-clientDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: runClient did not return after context cancel")
	}
	<-ifaceDone
}

// TestServerAcceptAndShutdown starts runServer on a loopback address and
// verifies it shuts down cleanly when ctx is cancelled.
func TestServerAcceptAndShutdown(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	iface := newTestIface(stdinR, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	ifaceDone := make(chan error, 1)
	go func() { ifaceDone <- iface.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	cfg := Config{
		Mode:       "server",
		ListenAddr: "127.0.0.1:0",
		MTU:        500,
	}

	serverDone := make(chan error, 1)
	go func() { serverDone <- runServer(ctx, cfg, iface, discardLogger()) }()
	time.Sleep(50 * time.Millisecond)

	cancel()
	_ = stdinW.Close()

	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: runServer did not return after context cancel")
	}
	<-ifaceDone
}

// TestErrorPropagation verifies that errors from runServer propagate even when
// ctx has not been cancelled — the ctxErr == nil check in main() would catch this
// and call os.Exit(1). Uses an invalid listen address to force an immediate error.
func TestErrorPropagation(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	iface := newTestIface(stdinR, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ifaceDone := make(chan error, 1)
	go func() { ifaceDone <- iface.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	cfg := Config{
		Mode:       "server",
		ListenAddr: "invalid-address-for-test",
		MTU:        500,
	}

	serverDone := make(chan error, 1)
	go func() { serverDone <- runServer(ctx, cfg, iface, discardLogger()) }()

	select {
	case err := <-serverDone:
		if err == nil {
			t.Fatal("expected non-nil error from runServer with invalid listen address")
		}
		// Context must not have been cancelled — this simulates the main() scenario
		// where ctxErr == nil means a real fatal error occurred.
		if ctx.Err() != nil {
			t.Fatalf("ctx was cancelled unexpectedly: %v", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: runServer did not return with invalid address")
	}
}
