package rnspipe

import (
	"bytes"
	"context"
	"errors"
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

	// Empty payload (FLAG+FLAG) should deliver an empty packet, matching Python
	// upstream which calls process_incoming(data_buffer) unconditionally.
	select {
	case pkt := <-dec.Packets():
		if len(pkt) != 0 {
			t.Fatalf("expected empty packet, got %x", pkt)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("expected empty packet from FLAG+FLAG frame")
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

	waitOnline(t, iface)

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
	gotPacket := make(chan []byte, 1)
	iface.OnSend(func(pkt []byte) error {
		select {
		case gotPacket <- append([]byte(nil), pkt...):
		default:
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- iface.Start(ctx)
	}()

	waitOnline(t, iface)

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
	select {
	case pkt := <-gotPacket:
		if !bytes.Equal(pkt, inPayload) {
			t.Fatalf("inbound: got %x, want %x", pkt, inPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound packet callback")
	}

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

// TestReceiveWhileOffline verifies that Receive returns ErrOffline when
// started=true but online=false (i.e. during the reconnect backoff window).
func TestReceiveWhileOffline(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:                stdinR,
		Stdout:               &stdout,
		ReconnectDelay:       200 * time.Millisecond,
		MaxReconnectAttempts: 2,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()
	waitOnline(t, iface)

	// Trigger EOF → interface goes offline, reconnector waits 200ms
	_ = stdinW.Close()

	deadline := time.After(2 * time.Second)
	for iface.IsOnline() {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for interface to go offline")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	if err := iface.Receive([]byte("test")); !errors.Is(err, ErrOffline) {
		t.Fatalf("expected ErrOffline while offline, got %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
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

	waitOnline(t, iface)

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

	waitOnline(t, iface)

	if err := iface.Start(ctx); err != ErrAlreadyStarted {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}

	cancel()
	_ = stdinW.Close()
	<-done
}

// TestEOFTriggersReconnect verifies that a clean EOF on stdin triggers a
// reconnect attempt rather than silently terminating the interface.
func TestEOFTriggersReconnect(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &stdout,
		// One attempt: readLoop fails with EOF → ErrMaxReconnectAttemptsReached.
		MaxReconnectAttempts: 1,
		ReconnectDelay:       10 * time.Millisecond,
	})

	var received [][]byte
	var mu sync.Mutex
	iface.OnSend(func(pkt []byte) error {
		mu.Lock()
		received = append(received, pkt)
		mu.Unlock()
		return nil
	})

	offlineSeen := make(chan struct{}, 1)
	iface.OnStatus(func(online bool) {
		if !online {
			select {
			case offlineSeen <- struct{}{}:
			default:
			}
		}
	})

	done := make(chan error, 1)
	go func() {
		done <- iface.Start(context.Background())
	}()

	// Write one valid HDLC frame then close stdin.
	enc := &Encoder{}
	payload := []byte("eof-test")
	if _, err := stdinW.Write(enc.Encode(payload)); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	_ = stdinW.Close()

	select {
	case err := <-done:
		if !errors.Is(err, ErrMaxReconnectAttemptsReached) {
			t.Fatalf("expected ErrMaxReconnectAttemptsReached, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Start to return")
	}

	// Verify the packet was delivered.
	mu.Lock()
	got := received
	mu.Unlock()
	if len(got) != 1 || !bytes.Equal(got[0], payload) {
		t.Fatalf("expected one packet %x, got %v", payload, got)
	}

	// Verify offline transition was signalled.
	select {
	case <-offlineSeen:
	default:
		t.Fatal("expected offline status transition")
	}

	if iface.IsOnline() {
		t.Fatal("expected interface offline after stop")
	}
}

// TestExitOnEOFTerminates verifies that ExitOnEOF=true causes Start to return
// ErrPipeClosed on clean stdin EOF without entering any reconnect delay.
func TestExitOnEOFTerminates(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:          stdinR,
		Stdout:         &stdout,
		ExitOnEOF:      true,
		ReconnectDelay: 10 * time.Second, // large delay — must not be reached
	})

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()
	waitOnline(t, iface)

	start := time.Now()
	_ = stdinW.Close() // clean EOF

	select {
	case err := <-done:
		if !errors.Is(err, ErrPipeClosed) {
			t.Fatalf("expected ErrPipeClosed, got %v", err)
		}
		// Must return before reconnect delay fires — verify no retry occurred.
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("took too long (%v); reconnect was attempted", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: Start did not return after EOF with ExitOnEOF=true")
	}
}

// TestDroppedPackets verifies that the Decoder counts dropped packets when the
// channel is full.
func TestDroppedPackets(t *testing.T) {
	enc := &Encoder{}
	frame1 := enc.Encode([]byte("pkt1"))
	frame2 := enc.Encode([]byte("pkt2"))

	// chanSize=1: first packet fills the channel, second is dropped.
	dec := NewDecoder(1064, 1)
	if _, err := dec.Write(frame1); err != nil {
		t.Fatalf("write frame1: %v", err)
	}
	if _, err := dec.Write(frame2); err != nil {
		t.Fatalf("write frame2: %v", err)
	}

	if got := dec.DroppedPackets(); got != 1 {
		t.Fatalf("expected 1 dropped packet, got %d", got)
	}
	dec.Close()
}

// TestCallbackRaceDetector registers OnSend/OnStatus concurrently with an
// active interface. Run with -race to verify no data races.
func TestCallbackRaceDetector(t *testing.T) {
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

	waitOnline(t, iface)

	// Register callbacks from a separate goroutine while Start is running.
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			iface.OnSend(func([]byte) error { return nil })
			iface.OnStatus(func(bool) {})
		}()
	}
	wg.Wait()

	cancel()
	_ = stdinW.Close()
	<-done
}

// TestShutdownNoGoroutineLeak verifies that cancelling the context causes
// readLoop to close stdin and wait for the io.Copy goroutine before returning.
// Run with -race to confirm no data race on the decoder after Start returns.
func TestShutdownNoGoroutineLeak(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinW.Close() }()
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

	waitOnline(t, iface)
	cancel() // fix should close stdinR and wait for io.Copy goroutine

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: goroutine likely leaked after context cancel")
	}
}

// TestRestartAfterStop verifies that Start can be called again on the same
// Interface after a previous Start has returned, without returning ErrAlreadyStarted.
func TestRestartAfterStop(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &stdout,
	})

	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan error, 1)
	go func() { done1 <- iface.Start(ctx1) }()

	waitOnline(t, iface)
	cancel1()

	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		_ = stdinW.Close()
		t.Fatal("first Start did not return")
	}
	_ = stdinW.Close()

	// After first Start returns, started should be false.
	// Start again with an already-cancelled context so it returns immediately.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	done2 := make(chan error, 1)
	go func() { done2 <- iface.Start(ctx2) }()

	select {
	case err := <-done2:
		if errors.Is(err, ErrAlreadyStarted) {
			t.Fatal("Start returned ErrAlreadyStarted after previous Stop")
		}
	case <-time.After(time.Second):
		t.Fatal("second Start did not return")
	}
}

// TestRestartRaceSetOnline verifies that cancelling Start() and immediately
// calling Start() again does not leave the interface in an inconsistent online
// state. Run with -race to catch data races.
func TestRestartRaceSetOnline(t *testing.T) {
	stdinR1, stdinW1 := io.Pipe()
	stdinR2, stdinW2 := io.Pipe()
	var stdout syncWriter

	iface := New(Config{
		Stdin:  stdinR1,
		Stdout: &stdout,
	})

	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan error, 1)
	go func() { done1 <- iface.Start(ctx1) }()

	waitOnline(t, iface)

	// Cancel first Start and wait for it to finish.
	cancel1()
	<-done1

	// Swap in a fresh stdin for the second Start.
	iface.config.Stdin = stdinR2

	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan error, 1)
	go func() { done2 <- iface.Start(ctx2) }()

	waitOnline(t, iface)

	// The interface should be online from the second Start.
	if !iface.IsOnline() {
		t.Fatal("expected online after restart")
	}

	// Clean up.
	cancel2()
	_ = stdinW1.Close()
	_ = stdinW2.Close()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second Start to return")
	}
}

// TestReceiveShortWrite verifies that Receive returns io.ErrShortWrite when
// the underlying writer accepts fewer bytes than provided (matching Python's
// IOError check in process_outgoing).
func TestReceiveShortWrite(t *testing.T) {
	stdinR, _ := io.Pipe()

	iface := New(Config{
		Stdin:  stdinR,
		Stdout: &shortWriter{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	err := iface.Receive([]byte("test"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}

	cancel()
	<-done
}

// shortWriter always reports writing one fewer byte than provided, with no error.
type shortWriter struct{}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
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
