package main

import (
	"bytes"
	"context"
	"errors"
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

// TestClientSendWhileDisconnected verifies that clientConn.send returns
// ErrOffline (not nil) when conn is nil, preventing silent packet drops.
func TestClientSendWhileDisconnected(t *testing.T) {
	cc := &clientConn{} // conn == nil
	err := cc.send([]byte("test"))
	if !errors.Is(err, rnspipe.ErrOffline) {
		t.Fatalf("expected ErrOffline, got %v", err)
	}
}

// TestClientConnectDisconnect starts runClient against a local listener,
// accepts the connection, then cancels ctx and verifies clean shutdown.
func TestClientConnectDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()
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
	go func() { clientDone <- runClient(ctx, cfg, iface, discardLogger(), make(chan struct{})) }()

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
	defer func() { _ = conn.Close() }()

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
	defer func() { _ = stdinW.Close() }()
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
	go func() { serverDone <- runServer(ctx, cfg, iface, discardLogger(), make(chan struct{})) }()
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

// TestServerMultiClientBroadcast starts runServer, connects two TCP clients,
// injects an HDLC packet via the pipe interface, and verifies both clients
// receive the decoded payload — confirming broadcast semantics.
func TestServerMultiClientBroadcast(t *testing.T) {
	// Pre-allocate an ephemeral port then release it for runServer to bind.
	ln0, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln0.Addr().String()
	_ = ln0.Close()

	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()
	stdoutR, stdoutW := io.Pipe()
	defer func() { _ = stdoutR.Close() }()
	go func() { _, _ = io.Copy(io.Discard, stdoutR) }() // drain pipe stdout

	iface := rnspipe.New(rnspipe.Config{
		Stdin:  stdinR,
		Stdout: stdoutW,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = iface.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	ready := make(chan struct{})
	go func() {
		_ = runServer(ctx, Config{Mode: "server", ListenAddr: addr, MTU: 500},
			iface, discardLogger(), ready)
	}()
	<-ready
	time.Sleep(30 * time.Millisecond)

	c1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c1.Close() }()
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	time.Sleep(20 * time.Millisecond)

	// Send packet from rnsd side via pipe stdin.
	var enc rnspipe.Encoder
	payload := []byte("broadcast-test")
	if _, err := stdinW.Write(enc.Encode(payload)); err != nil {
		t.Fatalf("write to stdin: %v", err)
	}

	readPkt := func(conn net.Conn) ([]byte, error) {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return nil, err
		}
		buf := make([]byte, 512)
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		dec := rnspipe.NewDecoder(tcpHWMTU, 4)
		if _, err := dec.Write(buf[:n]); err != nil {
			return nil, err
		}
		dec.Close()
		select {
		case pkt := <-dec.Packets():
			return pkt, nil
		default:
			return nil, errors.New("no packet decoded")
		}
	}

	pkt1, err := readPkt(c1)
	if err != nil || !bytes.Equal(pkt1, payload) {
		t.Fatalf("client1: err=%v pkt=%x", err, pkt1)
	}
	pkt2, err := readPkt(c2)
	if err != nil || !bytes.Equal(pkt2, payload) {
		t.Fatalf("client2: err=%v pkt=%x", err, pkt2)
	}
}

// TestErrorPropagation verifies that errors from runServer propagate even when
// ctx has not been cancelled — the ctxErr == nil check in main() would catch this
// and call os.Exit(1). Uses an invalid listen address to force an immediate error.
func TestErrorPropagation(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()
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
	go func() { serverDone <- runServer(ctx, cfg, iface, discardLogger(), make(chan struct{})) }()

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
