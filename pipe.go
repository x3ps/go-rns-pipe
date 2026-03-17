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
	decoder  *Decoder
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
	if config.HW_MTU == 0 {
		config.HW_MTU = defaults.HW_MTU
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
	iface.onSend = fn
}

// OnStatus registers a callback invoked on online/offline transitions.
func (iface *Interface) OnStatus(fn func(bool)) {
	iface.onStatus = fn
}

// Start begins reading HDLC-framed packets from config.Stdin. Decoded packets
// are delivered via the onSend callback. Start blocks until ctx is cancelled or
// an unrecoverable error occurs.
//
// The interface goes online immediately (no handshake), matching PipeInterface.py.
// On read errors, it attempts reconnection with exponential backoff.
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
	}()

	recon := &reconnector{
		baseDelay:   iface.config.ReconnectDelay,
		maxAttempts: iface.config.MaxReconnectAttempts,
		logger:      iface.logger,
		onStatus: func(online bool) {
			iface.setOnline(online)
		},
	}

	for {
		iface.setOnline(true)

		err := iface.readLoop(ctx)
		if err == nil || ctx.Err() != nil {
			iface.setOnline(false)
			return ctx.Err()
		}

		iface.logger.Warn("read loop exited", "error", err)
		iface.setOnline(false)

		// Attempt reconnection — the retry fn just re-enters the read loop.
		// If the underlying reader is a pipe to a subprocess, the provider
		// is responsible for restarting it and supplying a new reader.
		reconErr := recon.run(ctx, func() error {
			return iface.readLoop(ctx)
		})
		if reconErr != nil {
			return reconErr
		}
	}
}

// readLoop reads from stdin, feeds bytes into the HDLC decoder, and dispatches
// decoded packets via onSend. Returns on read error or context cancellation.
func (iface *Interface) readLoop(ctx context.Context) error {
	decoder := NewDecoder(iface.config.HW_MTU, iface.config.ReceiveBufferSize)
	iface.mu.Lock()
	iface.decoder = decoder
	iface.mu.Unlock()

	defer decoder.Close()

	// Read goroutine: feed stdin bytes into decoder.
	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(decoder, iface.config.Stdin)
		readErr <- err
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			// Drain any remaining packets before returning.
			iface.drainPackets(decoder)
			return err
		case pkt := <-decoder.Packets():
			if iface.onSend != nil {
				if err := iface.onSend(pkt); err != nil {
					iface.logger.Warn("onSend callback error", "error", err)
				}
			}
		}
	}
}

// drainPackets consumes any remaining packets from the decoder channel.
func (iface *Interface) drainPackets(decoder *Decoder) {
	for {
		select {
		case pkt := <-decoder.Packets():
			if iface.onSend != nil {
				_ = iface.onSend(pkt)
			}
		default:
			return
		}
	}
}

// Receive encodes a packet with HDLC framing and writes it to config.Stdout.
// This is called when the user wants to send a packet into the pipe (towards rnsd).
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

func (iface *Interface) setOnline(online bool) {
	iface.mu.Lock()
	changed := iface.online != online
	iface.online = online
	iface.mu.Unlock()

	if changed && iface.onStatus != nil {
		iface.onStatus(online)
	}
}
