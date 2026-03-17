package main

import (
	"context"
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
	t.Cleanup(func() { ln.Close() })

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
	t.Cleanup(func() { client.Close() })

	select {
	case server = <-connCh:
		t.Cleanup(func() { server.Close() })
	case <-time.After(2 * time.Second):
		t.Fatal("loopbackConn: accept timeout")
	}
	return server, client
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
