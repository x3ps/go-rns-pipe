// Package rnspipe implements the RNS PipeInterface protocol in Go.
//
// It provides HDLC-framed communication over any io.Reader/io.Writer pair,
// matching the behavior of Reticulum's PipeInterface.py.
package rnspipe

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Interface implements the RNS PipeInterface protocol. It reads HDLC-framed
// packets from Stdin and writes HDLC-framed packets to Stdout.
type Interface struct {
	config   Config
	mu       sync.RWMutex
	online   bool
	started  bool
	encoder  Encoder
	onSend   func([]byte) error // called when a decoded packet arrives from stdin
	onStatus func(bool)         // called on online/offline transitions
	logger   *slog.Logger
	cancelFn context.CancelFunc
}

// New creates an Interface with the given config, applying defaults where needed.
func New(config Config) *Interface {
	defaults := DefaultConfig()

	if config.Name == "" {
		config.Name = defaults.Name
	}
	if config.MTU == 0 {
		config.MTU = defaults.MTU
	}
	if config.HWMTU == 0 {
		config.HWMTU = defaults.HWMTU
	}
	if config.Bitrate == 0 {
		config.Bitrate = defaults.Bitrate
	}
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = defaults.ReconnectDelay
	}
	if config.ReceiveBufferSize == 0 {
		config.ReceiveBufferSize = defaults.ReceiveBufferSize
	}

	// Clamp negative values to defaults to prevent downstream panics
	// (e.g. make(chan []byte, negative) in NewDecoder).
	if config.MTU < 0 {
		config.MTU = defaults.MTU
	}
	if config.HWMTU < 0 {
		config.HWMTU = defaults.HWMTU
	}
	if config.Bitrate < 0 {
		config.Bitrate = defaults.Bitrate
	}
	if config.ReconnectDelay < 0 {
		config.ReconnectDelay = defaults.ReconnectDelay
	}
	if config.ReceiveBufferSize < 0 {
		config.ReceiveBufferSize = defaults.ReceiveBufferSize
	}

	if config.Stdin == nil {
		config.Stdin = os.Stdin
	}
	if config.Stdout == nil {
		config.Stdout = os.Stdout
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: config.LogLevel,
		}))
	}

	return &Interface{
		config: config,
		logger: logger.With("interface", config.Name),
	}
}

// OnSend registers a callback invoked for each decoded packet read from stdin.
func (iface *Interface) OnSend(fn func([]byte) error) {
	iface.mu.Lock()
	iface.onSend = fn
	iface.mu.Unlock()
}

// OnStatus registers a callback invoked on online/offline transitions.
func (iface *Interface) OnStatus(fn func(bool)) {
	iface.mu.Lock()
	iface.onStatus = fn
	iface.mu.Unlock()
}

// Start begins reading HDLC-framed packets from config.Stdin. Decoded packets
// are delivered via the onSend callback. Start blocks until ctx is cancelled or
// an unrecoverable error occurs.
//
// The interface goes online immediately (no handshake), matching PipeInterface.py.
// On read errors or clean EOF, it attempts reconnection with exponential backoff.
func (iface *Interface) Start(ctx context.Context) error {
	iface.mu.Lock()
	if iface.started {
		iface.mu.Unlock()
		return ErrAlreadyStarted
	}
	iface.started = true
	ctx, iface.cancelFn = context.WithCancel(ctx)
	iface.mu.Unlock()

	defer func() {
		iface.mu.Lock()
		iface.started = false
		iface.cancelFn = nil
		iface.mu.Unlock()
		iface.setOnline(false) // safety net
	}()

	recon := &reconnector{
		baseDelay:   iface.config.ReconnectDelay,
		maxAttempts: iface.config.MaxReconnectAttempts,
		logger:      iface.logger,
	}

	return recon.run(ctx, func() error {
		iface.setOnline(true)
		err := iface.readLoop(ctx)
		iface.setOnline(false)
		return err // nil = ctx cancelled; io.EOF / other error = reconnect
	})
}

// readLoop reads from stdin, feeds bytes into the HDLC decoder, and dispatches
// decoded packets via onSend. Returns nil on context cancellation, io.EOF on
// clean EOF (triggers reconnect), or another error on read failure.
//
// On context cancellation, if Stdin implements io.Closer it is closed to
// unblock the io.Copy goroutine, and readLoop waits for the goroutine to exit.
// os.Stdin is explicitly excluded from this close: closing os.Stdin would affect
// the entire process and is unexpected for a long-running daemon. If Stdin does
// not implement io.Closer (and is not os.Stdin), the goroutine will remain
// blocked until the reader is otherwise closed.
func (iface *Interface) readLoop(ctx context.Context) error {
	decoder := NewDecoder(iface.config.HWMTU, iface.config.ReceiveBufferSize)

	// Read goroutine: feed stdin bytes into decoder.
	// decoder.Close() is called here so the packets channel is only closed
	// after all bytes have been processed — preventing a write-to-closed-channel panic.
	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(decoder, iface.config.Stdin)
		decoder.Close()
		readErr <- err
	}()

	pkts := decoder.Packets()
	for {
		select {
		case <-ctx.Done():
			if iface.config.Stdin != os.Stdin {
				if closer, ok := iface.config.Stdin.(io.Closer); ok {
					_ = closer.Close()
					<-readErr // wait for io.Copy goroutine to exit
				}
			}
			return nil
		case err := <-readErr:
			// Drain any remaining packets before returning.
			iface.drainPackets(decoder)
			if dropped := decoder.DroppedPackets(); dropped > 0 {
				iface.logger.Warn("packets dropped during read", "count", dropped)
			}
			if err == nil {
				// Clean EOF: remote closed the pipe. Signal reconnect.
				return io.EOF
			}
			return err
		case pkt, ok := <-pkts:
			if !ok {
				// Channel closed; disable this case and wait for readErr.
				pkts = nil
				continue
			}
			iface.mu.RLock()
			cb := iface.onSend
			iface.mu.RUnlock()
			if cb != nil {
				if err := cb(pkt); err != nil {
					iface.logger.Warn("onSend callback error", "error", err)
				}
			}
		}
	}
}

// drainPackets consumes any remaining packets from the decoder channel.
func (iface *Interface) drainPackets(decoder *Decoder) {
	iface.mu.RLock()
	cb := iface.onSend
	iface.mu.RUnlock()
	for {
		select {
		case pkt, ok := <-decoder.Packets():
			if !ok {
				return // channel closed
			}
			if cb != nil {
				_ = cb(pkt)
			}
		default:
			return
		}
	}
}

// Receive encodes packet with HDLC framing and writes it to config.Stdout (towards rnsd).
// Despite the name (which matches the Python PipeInterface API), this is outbound from
// the caller's perspective: use it to inject a packet into the RNS pipe.
func (iface *Interface) Receive(packet []byte) error {
	iface.mu.RLock()
	started := iface.started
	iface.mu.RUnlock()

	if !started {
		return ErrNotStarted
	}

	frame := iface.encoder.Encode(packet)

	iface.mu.Lock()
	_, err := iface.config.Stdout.Write(frame)
	iface.mu.Unlock()

	return err
}

// IsOnline returns whether the interface is currently online.
func (iface *Interface) IsOnline() bool {
	iface.mu.RLock()
	defer iface.mu.RUnlock()
	return iface.online
}

// Name returns the interface name.
func (iface *Interface) Name() string {
	return iface.config.Name
}

// MTU returns the configured MTU.
func (iface *Interface) MTU() int {
	return iface.config.MTU
}

// HWMTU returns the configured hardware MTU.
func (iface *Interface) HWMTU() int {
	return iface.config.HWMTU
}

func (iface *Interface) setOnline(online bool) {
	iface.mu.Lock()
	changed := iface.online != online
	iface.online = online
	cb := iface.onStatus
	iface.mu.Unlock()

	if changed && cb != nil {
		cb(online)
	}
}
