package rnspipe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitOnline polls until iface.IsOnline() returns true or the deadline expires.
func waitOnline(t *testing.T, iface *Interface) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for !iface.IsOnline() {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for online")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// newTestPipe creates an Interface wired to io.Pipe pairs for testing.
// Returns the interface, the writer end of stdin (to inject data), and
// the reader end of stdout (to read outbound frames).
func newTestPipe(t *testing.T, opts ...func(*Config)) (*Interface, *io.PipeWriter, *io.PipeReader) {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	cfg := Config{
		Stdin:          stdinR,
		Stdout:         stdoutW,
		ReconnectDelay: 10 * time.Millisecond,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	iface := New(cfg)
	t.Cleanup(func() {
		_ = stdinW.Close()
		_ = stdoutR.Close()
	})
	return iface, stdinW, stdoutR
}

func TestOnSendCallbackError(t *testing.T) {
	t.Parallel()

	iface, stdinW, _ := newTestPipe(t)

	callbackErr := errors.New("callback failed")
	var callCount atomic.Int32

	iface.OnSend(func(pkt []byte) error {
		callCount.Add(1)
		return callbackErr
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	// Send two frames — both should invoke the callback despite errors.
	enc := &Encoder{}
	for _, payload := range []string{"pkt1", "pkt2"} {
		if _, err := stdinW.Write(enc.Encode([]byte(payload))); err != nil {
			t.Fatalf("write %s: %v", payload, err)
		}
	}

	// Wait for both callbacks.
	deadline := time.After(2 * time.Second)
	for callCount.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("expected 2 callbacks, got %d", callCount.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

func TestDroppedPacketLogging(t *testing.T) {
	t.Parallel()

	// Build all frames into a single buffer so the decoder processes them
	// in one Write call — before the readLoop select can drain the channel.
	enc := &Encoder{}
	var allFrames bytes.Buffer
	for i := range 5 {
		allFrames.Write(enc.Encode([]byte(fmt.Sprintf("drop%d", i))))
	}

	var logBuf syncBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	iface := New(Config{
		Stdin:                bytes.NewReader(allFrames.Bytes()),
		Stdout:               &bytes.Buffer{},
		ReceiveBufferSize:    1,
		ReconnectDelay:       10 * time.Millisecond,
		Logger:               logger,
		MaxReconnectAttempts: 1,
	})
	// No-op handler: drops happen inside the decoder (channel capacity 1)
	// before readLoop can consume, so the log warning is still triggered.
	iface.OnSend(func([]byte) error { return nil })

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Start to return")
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("packets dropped")) {
		t.Errorf("expected 'packets dropped' warning in log, got: %s", string(logBuf.Bytes()))
	}
}

// syncBuffer is a thread-safe bytes.Buffer for concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}

func TestDrainPacketsDeliversToCallback(t *testing.T) {
	t.Parallel()

	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	iface := New(Config{
		Stdin:                stdinR,
		Stdout:               &stdout,
		ReconnectDelay:       10 * time.Millisecond,
		MaxReconnectAttempts: 1,
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	var received [][]byte
	var mu sync.Mutex
	iface.OnSend(func(pkt []byte) error {
		mu.Lock()
		received = append(received, append([]byte(nil), pkt...))
		mu.Unlock()
		return nil
	})

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()

	waitOnline(t, iface)

	// Write a frame and immediately close stdin to trigger drainPackets.
	enc := &Encoder{}
	payload := []byte("drained")
	if _, err := stdinW.Write(enc.Encode(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Small delay so the packet is processed before EOF closes the decoder.
	time.Sleep(50 * time.Millisecond)
	_ = stdinW.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, pkt := range received {
		if bytes.Equal(pkt, payload) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %x in received packets, got %v", payload, received)
	}
}

func TestDrainCallbackErrorLogging(t *testing.T) {
	t.Parallel()

	// Build a single frame followed by EOF so drainPackets is called.
	enc := &Encoder{}
	frame := enc.Encode([]byte("drain-err-test"))

	var logBuf syncBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	iface := New(Config{
		Stdin:                bytes.NewReader(frame),
		Stdout:               &bytes.Buffer{},
		ReceiveBufferSize:    1,
		ReconnectDelay:       10 * time.Millisecond,
		MaxReconnectAttempts: 1,
		Logger:               logger,
	})

	callbackErr := errors.New("drain callback failed")
	iface.OnSend(func([]byte) error {
		return callbackErr
	})

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Start to return")
	}

	logOutput := string(logBuf.Bytes())
	if !bytes.Contains(logBuf.Bytes(), []byte("onSend callback error")) &&
		!bytes.Contains(logBuf.Bytes(), []byte("drain")) {
		t.Errorf("expected drain callback error warning in log, got: %s", logOutput)
	}
}

// errorReader returns a configurable error after writing n bytes.
type errorReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestReadErrorNonEOF(t *testing.T) {
	t.Parallel()

	customErr := errors.New("connection reset")
	enc := &Encoder{}
	frame := enc.Encode([]byte("before-error"))

	iface := New(Config{
		Stdin:                &errorReader{data: frame, err: customErr},
		Stdout:               &bytes.Buffer{},
		ReconnectDelay:       10 * time.Millisecond,
		MaxReconnectAttempts: 1,
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	iface.OnSend(func([]byte) error { return nil })

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()

	select {
	case err := <-done:
		// The reconnector should exhaust attempts and return ErrMaxReconnectAttemptsReached.
		if !errors.Is(err, ErrMaxReconnectAttemptsReached) {
			t.Fatalf("expected ErrMaxReconnectAttemptsReached, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// readerOnly wraps an io.Reader to strip any io.Closer implementation.
type readerOnly struct {
	io.Reader
}

func TestNonCloserStdin(t *testing.T) {
	t.Parallel()

	// Use a reader that doesn't implement io.Closer.
	// On context cancel, readLoop should return without waiting for
	// the io.Copy goroutine (since it can't close stdin).
	r, w := io.Pipe()
	stdin := &readerOnly{Reader: r}

	iface := New(Config{
		Stdin:  stdin,
		Stdout: &bytes.Buffer{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	iface.OnSend(func([]byte) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: readLoop did not return for non-Closer stdin")
	}

	// Clean up the blocked goroutine.
	_ = w.Close()
}

func TestOnStatusTransitions(t *testing.T) {
	t.Parallel()

	iface, stdinW, _ := newTestPipe(t)
	iface.OnSend(func([]byte) error { return nil })

	var transitions []bool
	var mu sync.Mutex
	iface.OnStatus(func(online bool) {
		mu.Lock()
		transitions = append(transitions, online)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	cancel()
	_ = stdinW.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}

	mu.Lock()
	defer mu.Unlock()

	// Expect at least [true, false] (online then offline).
	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %v", transitions)
	}
	if transitions[0] != true {
		t.Errorf("first transition should be true (online), got %v", transitions[0])
	}
	// Last transition should be false (offline).
	if transitions[len(transitions)-1] != false {
		t.Errorf("last transition should be false (offline), got %v", transitions[len(transitions)-1])
	}
}

// respawningReader wraps a slice of io.Readers. Each Read call forwards to the
// current reader; on EOF the index advances so the next readLoop iteration
// gets a fresh reader.
type respawningReader struct {
	readers []io.Reader
	idx     atomic.Int32
}

func (r *respawningReader) Read(p []byte) (int, error) {
	i := int(r.idx.Load())
	if i >= len(r.readers) {
		return 0, io.EOF
	}
	n, err := r.readers[i].Read(p)
	if err == io.EOF {
		r.idx.Add(1)
	}
	return n, err
}

func TestReconnectWithNewStdin(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	// Two readers, each containing one HDLC frame followed by EOF.
	frame1 := enc.Encode([]byte("reconnect-pkt-1"))
	frame2 := enc.Encode([]byte("reconnect-pkt-2"))

	rr := &respawningReader{
		readers: []io.Reader{
			bytes.NewReader(frame1),
			bytes.NewReader(frame2),
		},
	}

	var logBuf syncBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	iface := New(Config{
		Stdin:                rr,
		Stdout:               &bytes.Buffer{},
		ReconnectDelay:       10 * time.Millisecond,
		MaxReconnectAttempts: 3,
		Logger:               logger,
	})

	var received [][]byte
	var mu sync.Mutex
	allReceived := make(chan struct{}, 1)

	iface.OnSend(func(pkt []byte) error {
		mu.Lock()
		received = append(received, append([]byte(nil), pkt...))
		if len(received) == 2 {
			select {
			case allReceived <- struct{}{}:
			default:
			}
		}
		mu.Unlock()
		return nil
	})

	var transitions []bool
	var tmu sync.Mutex
	iface.OnStatus(func(online bool) {
		tmu.Lock()
		transitions = append(transitions, online)
		tmu.Unlock()
	})

	done := make(chan error, 1)
	go func() { done <- iface.Start(context.Background()) }()

	// Wait for both packets or timeout.
	select {
	case <-allReceived:
	case <-time.After(5 * time.Second):
		mu.Lock()
		n := len(received)
		mu.Unlock()
		t.Fatalf("timeout: received %d/2 packets", n)
	}

	// Wait for Start to finish (will exhaust reconnects after both readers EOF).
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Start to return")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected 2 packets, got %d", len(received))
	}
	if !bytes.Equal(received[0], []byte("reconnect-pkt-1")) {
		t.Errorf("pkt1: got %q, want %q", received[0], "reconnect-pkt-1")
	}
	if !bytes.Equal(received[1], []byte("reconnect-pkt-2")) {
		t.Errorf("pkt2: got %q, want %q", received[1], "reconnect-pkt-2")
	}

	// Check transitions: should see online->offline->online->offline pattern.
	tmu.Lock()
	defer tmu.Unlock()
	if len(transitions) < 4 {
		t.Fatalf("expected at least 4 status transitions, got %v", transitions)
	}
	if transitions[0] != true || transitions[1] != false || transitions[2] != true || transitions[3] != false {
		t.Errorf("expected [true,false,true,false,...], got %v", transitions)
	}
}

func TestConcurrentReceiveAndInbound(t *testing.T) {
	t.Parallel()

	iface, stdinW, stdoutR := newTestPipe(t)

	const inboundCount = 50
	const outboundGoroutines = 10
	const outboundPerGoroutine = 5

	var receivedCount atomic.Int32
	allInbound := make(chan struct{}, 1)

	iface.OnSend(func(pkt []byte) error {
		if receivedCount.Add(1) == inboundCount {
			select {
			case allInbound <- struct{}{}:
			default:
			}
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	// Drain stdout in background.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdoutR.Read(buf); err != nil {
				return
			}
		}
	}()

	// Launch concurrent outbound Receive() calls.
	var wg sync.WaitGroup
	for i := range outboundGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range outboundPerGoroutine {
				_ = iface.Receive([]byte(fmt.Sprintf("out-%d-%d", i, j)))
			}
		}()
	}

	// Simultaneously write inbound HDLC frames.
	enc := &Encoder{}
	for i := range inboundCount {
		frame := enc.Encode([]byte(fmt.Sprintf("in-%d", i)))
		if _, err := stdinW.Write(frame); err != nil {
			t.Fatalf("write inbound frame %d: %v", i, err)
		}
	}

	wg.Wait()

	select {
	case <-allInbound:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout: received %d/%d inbound packets", receivedCount.Load(), inboundCount)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

func TestFullRoundTrip(t *testing.T) {
	t.Parallel()

	iface, stdinW, stdoutR := newTestPipe(t)

	var received [][]byte
	var mu sync.Mutex
	allReceived := make(chan struct{}, 1)
	const packetCount = 10

	iface.OnSend(func(pkt []byte) error {
		mu.Lock()
		received = append(received, append([]byte(nil), pkt...))
		if len(received) == packetCount {
			select {
			case allReceived <- struct{}{}:
			default:
			}
		}
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	// Read outbound frames in background.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := stdoutR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Send 10 packets outbound via Receive.
	for i := range packetCount {
		if err := iface.Receive([]byte(fmt.Sprintf("outbound-%d", i))); err != nil {
			t.Errorf("Receive %d: %v", i, err)
		}
	}

	// Send 10 packets inbound via stdin.
	enc := &Encoder{}
	for i := range packetCount {
		frame := enc.Encode([]byte(fmt.Sprintf("inbound-%d", i)))
		if _, err := stdinW.Write(frame); err != nil {
			t.Fatalf("write frame %d: %v", i, err)
		}
	}

	select {
	case <-allReceived:
	case <-time.After(5 * time.Second):
		mu.Lock()
		n := len(received)
		mu.Unlock()
		t.Fatalf("timeout: received %d/%d packets", n, packetCount)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

func TestFullRoundTripConcurrent(t *testing.T) {
	t.Parallel()

	iface, stdinW, stdoutR := newTestPipe(t)

	const goroutines = 10
	var receivedCount atomic.Int32
	allReceived := make(chan struct{}, 1)

	iface.OnSend(func(pkt []byte) error {
		if receivedCount.Add(1) == goroutines {
			select {
			case allReceived <- struct{}{}:
			default:
			}
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- iface.Start(ctx) }()

	waitOnline(t, iface)

	// Drain stdout in background.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := stdoutR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// 10 goroutines each send 1 inbound packet.
	enc := &Encoder{}
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frame := enc.Encode([]byte(fmt.Sprintf("concurrent-%d", i)))
			_, _ = stdinW.Write(frame)
		}()
	}
	wg.Wait()

	select {
	case <-allReceived:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout: received %d/%d", receivedCount.Load(), goroutines)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}
