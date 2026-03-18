package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// loopbackConn dials a local listener and returns the server-side connection.
func loopbackConn(t *testing.T) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	connCh := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			connCh <- c
		}
	}()

	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case server = <-connCh:
		t.Cleanup(func() { _ = server.Close() })
	case <-time.After(2 * time.Second):
		t.Fatal("loopbackConn: accept timeout")
	}
	return server, client
}

// shortWriteConn is a net.Conn stub that returns a partial write with no error.
type shortWriteConn struct {
	net.Conn
	written int
}

func (c *shortWriteConn) Write(b []byte) (int, error)        { return c.written, nil }
func (c *shortWriteConn) SetWriteDeadline(time.Time) error   { return nil }

// TestWritePacketShortWrite verifies that writePacket returns io.ErrShortWrite
// when the underlying conn.Write returns n < len(frame) with no error.
func TestWritePacketShortWrite(t *testing.T) {
	conn := &shortWriteConn{written: 0}
	var enc rnspipe.Encoder
	err := writePacket(conn, &enc, []byte{0x01, 0x02, 0x03})
	if err != io.ErrShortWrite {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

// TestReadPacketsCancelClean verifies that cancelling ctx causes readPackets to
// close the connection, wait for the inner io.Copy goroutine, and return promptly.
func TestReadPacketsCancelClean(t *testing.T) {
	server, _ := loopbackConn(t)

	packets := make(chan []byte, 64)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- readPackets(ctx, server, 1064, packets)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: readPackets did not return after context cancel — goroutine likely leaked")
	}
}

// TestReadPacketsLargePacket is a regression test verifying that readPackets
// decodes a packet close to tcpHWMTU without truncation.
// Prior bug: iface.HWMTU() (1064) was passed, silently truncating TCP packets
// larger than 1064 bytes received from Python TCPInterface peers.
func TestReadPacketsLargePacket(t *testing.T) {
	server, client := loopbackConn(t)

	const payloadSize = tcpHWMTU - 64
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i & 0xFF)
	}

	packets := make(chan []byte, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- readPackets(ctx, server, tcpHWMTU, packets)
	}()

	var enc rnspipe.Encoder
	if _, err := client.Write(enc.Encode(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = client.Close()

	select {
	case pkt := <-packets:
		if len(pkt) != payloadSize {
			t.Fatalf("expected %d bytes, got %d (packet truncated)", payloadSize, len(pkt))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for large packet")
	}
	<-done
}

// TestSetTCPSocketOptions validates the socket options code path doesn't panic
// on a real connection.
func TestSetTCPSocketOptions(t *testing.T) {
	server, client := loopbackConn(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	setTCPSocketOptions(client.(*net.TCPConn), logger)  // must not panic
	setTCPSocketOptions(server.(*net.TCPConn), logger)
}

// TestMalformedShortFrame verifies that a truncated HDLC frame (FLAG byte but no
// closing FLAG) is silently discarded: no packet is emitted and no panic occurs.
func TestMalformedShortFrame(t *testing.T) {
	server, client := loopbackConn(t)

	packets := make(chan []byte, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- readPackets(ctx, server, tcpHWMTU, packets)
	}()

	// Write a FLAG start byte followed by payload bytes but no closing FLAG.
	truncated := []byte{rnspipe.HDLCFlag, 0x01, 0x02, 0x03}
	if _, err := client.Write(truncated); err != nil {
		t.Fatalf("write truncated frame: %v", err)
	}
	_ = client.Close() // EOF — causes readPackets to return

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for readPackets to return")
	}

	// No packet should have been emitted.
	select {
	case pkt := <-packets:
		t.Fatalf("unexpected packet from truncated frame: %x", pkt)
	default:
	}
}

// TestReadPacketsChannelFull verifies that cancelling ctx does not deadlock when
// the packets channel is full and readPackets is blocked on a send.
func TestReadPacketsChannelFull(t *testing.T) {
	server, client := loopbackConn(t)

	// Small capacity so the channel fills after just a few packets.
	packets := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- readPackets(ctx, server, 1064, packets)
	}()

	// Write enough HDLC frames to saturate the packets channel.
	var enc rnspipe.Encoder
	for i := range 10 {
		if _, err := client.Write(enc.Encode([]byte{byte(i)})); err != nil {
			break
		}
	}

	// Wait for readPackets to fill the channel and block on the next send.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// returned — no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: readPackets did not return after cancel with full packets channel")
	}
}
