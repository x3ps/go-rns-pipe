package rnspipe

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

func TestHDLCEncodeDecode(t *testing.T) {
	payload := []byte("hello reticulum")
	enc := &Encoder{}
	frame := enc.Encode(payload)

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatal(err)
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, payload) {
			t.Fatalf("got %x, want %x", pkt, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestHDLCByteStuffing(t *testing.T) {
	// Payload contains both special bytes.
	payload := []byte{0x01, HDLCFlag, 0x02, HDLCEscape, 0x03}
	enc := &Encoder{}
	frame := enc.Encode(payload)

	// Frame should not contain bare FLAG bytes except at boundaries.
	inner := frame[1 : len(frame)-1]
	for i, b := range inner {
		if b == HDLCFlag {
			t.Fatalf("bare FLAG at inner offset %d", i)
		}
	}

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatal(err)
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, payload) {
			t.Fatalf("got %x, want %x", pkt, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestHDLCEmptyPacket(t *testing.T) {
	enc := &Encoder{}
	frame := enc.Encode([]byte{})

	// Should be just FLAG FLAG.
	if len(frame) != 2 || frame[0] != HDLCFlag || frame[1] != HDLCFlag {
		t.Fatalf("unexpected frame for empty packet: %x", frame)
	}

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatal(err)
	}

	// Empty payload produces no packet (decoder skips zero-length buffers).
	select {
	case pkt := <-dec.Packets():
		t.Fatalf("expected no packet, got %x", pkt)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestInterfaceStartStop(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &stdout,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- iface.Start(ctx)
	}()

	// Give it a moment to start.
	time.Sleep(50 * time.Millisecond)
	if !iface.IsOnline() {
		t.Fatal("expected online after start")
	}

	cancel()
	_ = stdinW.Close()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}

	if iface.IsOnline() {
		t.Fatal("expected offline after stop")
	}
}

func TestReceiveSend(t *testing.T) {
	// stdin: we write HDLC frames that the interface reads.
	stdinR, stdinW := io.Pipe()
	// stdout: the interface writes HDLC frames here.
	stdoutR, stdoutW := io.Pipe()

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: stdoutW,
	})

	// Capture packets decoded from stdin.
	var received [][]byte
	var mu sync.Mutex
	iface.OnSend(func(pkt []byte) error {
		mu.Lock()
		received = append(received, pkt)
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- iface.Start(ctx)
	}()

	// Wait for online.
	time.Sleep(50 * time.Millisecond)

	// Test outbound: Receive() should write HDLC frame to stdout.
	outPayload := []byte("outbound packet")
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := stdoutR.Read(buf)
		readDone <- buf[:n]
	}()

	if err := iface.Receive(outPayload); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	select {
	case frame := <-readDone:
		// Decode the frame to verify.
		dec := NewDecoder(1064, 1)
		defer dec.Close()
		if _, err := dec.Write(frame); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		select {
		case pkt := <-dec.Packets():
			if !bytes.Equal(pkt, outPayload) {
				t.Fatalf("outbound: got %x, want %x", pkt, outPayload)
			}
		default:
			t.Fatal("no packet decoded from outbound frame")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading stdout")
	}

	// Test inbound: write HDLC frame to stdin, expect onSend callback.
	enc := &Encoder{}
	inPayload := []byte("inbound packet")
	frame := enc.Encode(inPayload)
	if _, err := stdinW.Write(frame); err != nil {
		t.Fatalf("write to stdin: %v", err)
	}

	// Wait for callback.
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if len(received) != 1 || !bytes.Equal(received[0], inPayload) {
		t.Fatalf("inbound: got %v, want [%x]", received, inPayload)
	}
	mu.Unlock()

	cancel()
	_ = stdinW.Close()
	<-done
}

func TestReceiveOnStopped(t *testing.T) {
	iface := New(Config{})

	if err := iface.Receive([]byte("test")); err != ErrNotStarted {
		t.Fatalf("expected ErrNotStarted, got %v", err)
	}
}

func TestConcurrentReceive(t *testing.T) {
	stdinR, _ := io.Pipe()
	var stdout syncWriter

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &stdout,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- iface.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Fire concurrent Receive calls.
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = iface.Receive([]byte{byte(i)})
		}()
	}
	wg.Wait()

	// Verify all frames arrived (decode stdout).
	cancel()

	data := stdout.Bytes()
	dec := NewDecoder(1064, 64)
	defer dec.Close()
	if _, err := dec.Write(data); err != nil {
		t.Fatalf("decode data: %v", err)
	}

	count := 0
	for {
		select {
		case <-dec.Packets():
			count++
		default:
			goto done2
		}
	}
done2:
	if count != 20 {
		t.Fatalf("expected 20 packets, got %d", count)
	}
}

func TestNameAndMTU(t *testing.T) {
	iface := New(Config{Name: "test-iface", MTU: 1200})
	if iface.Name() != "test-iface" {
		t.Fatalf("Name: got %q, want %q", iface.Name(), "test-iface")
	}
	if iface.MTU() != 1200 {
		t.Fatalf("MTU: got %d, want %d", iface.MTU(), 1200)
	}
}

func TestAlreadyStarted(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &stdout,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- iface.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	if err := iface.Start(ctx); err != ErrAlreadyStarted {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}

	cancel()
	_ = stdinW.Close()
	<-done
}

// syncWriter is a thread-safe bytes.Buffer for concurrent writes.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Bytes()
}
