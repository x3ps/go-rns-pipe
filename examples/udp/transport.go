package main

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// hwMTU is the buffer size for reading UDP datagrams.
// Matches PipeInterface.py HWMTU = 1064.
const hwMTU = 1064

// Transport bridges a rnspipe.Interface (HDLC/pipe to rnsd) with a UDP socket.
type Transport struct {
	config     Config
	logger     *slog.Logger
	pipeConfig rnspipe.Config // zero value = defaults (os.Stdin/os.Stdout); tests inject io.Pipe pairs
}

// NewTransport creates a Transport with the given config and logger.
func NewTransport(cfg Config, logger *slog.Logger) *Transport {
	return &Transport{config: cfg, logger: logger}
}

// Start runs the UDP transport until ctx is cancelled or an unrecoverable error
// occurs. It connects to rnsd via stdin/stdout and forwards packets over UDP.
//
// Step 1: Resolve addresses.
// Step 2: Create rnspipe.Interface.
// Step 3: Register status callback.
// Step 4: Register OnSend callback.
// Step 5: Start pipe interface goroutine (pipeDone + loopCancel).
// Step 6: Start drop-counter logger.
// Step 7: UDP socket loop (reconnects on socket error, exits on loopCtx cancel).
// Step 8: Return pipe interface error.
func (t *Transport) Start(ctx context.Context) error {
	// Step 1: Resolve local listen address only — fail fast on bad config.
	// peerAddr is resolved lazily inside the socket loop to tolerate transient
	// DNS failures (e.g., peer container not yet started in Docker).
	listenAddr, err := net.ResolveUDPAddr("udp", t.config.ListenAddr)
	if err != nil {
		return err
	}

	// Step 2: Create rnspipe.Interface connected to rnsd via stdin/stdout.
	// t.pipeConfig provides a testability seam: tests inject io.Pipe pairs
	// via Stdin/Stdout; production uses os.Stdin/os.Stdout (zero-value defaults).
	pipeCfg := t.pipeConfig
	pipeCfg.Name = t.config.Name
	pipeCfg.MTU = t.config.MTU
	pipeCfg.LogLevel = t.config.LogLevel
	pipeCfg.ExitOnEOF = true
	iface := rnspipe.New(pipeCfg)

	// Step 3: Register status callback — log online/offline transitions.
	iface.OnStatus(func(online bool) {
		if online {
			t.logger.Info("pipe interface online")
		} else {
			t.logger.Warn("pipe interface offline")
		}
	})

	// conn, peerAddr, and connMu protect access to the active UDP socket and
	// resolved peer address across the OnSend callback and the socket loop.
	// Both are read/written under the same lock for atomicity.
	var (
		connMu   sync.RWMutex
		conn     *net.UDPConn
		peerAddr *net.UDPAddr
	)

	// Step 4: Register OnSend BEFORE Start — forward pipe packets to UDP peer.
	// Must be registered before iface.Start so no decoded packets are silently
	// dropped (pipe.go skips delivery when cb == nil).
	// The callback reads conn and peerAddr under connMu so it always sees a
	// consistent pair.
	// See: UDPInterface.py:process_outgoing
	var dropped atomic.Int64
	iface.OnSend(func(pkt []byte) error {
		connMu.RLock()
		c := conn
		peer := peerAddr
		connMu.RUnlock()

		if c == nil {
			// Socket is being replaced — drop silently.
			dropped.Add(1)
			return nil
		}
		if len(pkt) > t.config.MTU {
			dropped.Add(1)
			return nil
		}
		if peer == nil {
			dropped.Add(1)
			return nil
		}
		_, err := c.WriteTo(pkt, peer)
		return err
	})

	// Step 5: Start pipe interface in background; cancel socket loop when pipe exits.
	pipeDone := make(chan error, 1)
	loopCtx, loopCancel := context.WithCancel(ctx)
	defer loopCancel()

	go func() {
		pipeDone <- iface.Start(ctx) // uses original ctx, not loopCtx
		loopCancel()                 // kill socket loop when pipe exits
	}()

	// Step 6: Start drop-counter logger goroutine.
	// Oversized or mid-reconnect drops are counted and logged every 30s.
	// Uses loopCtx so it exits when the pipe exits, not only on external cancel.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if n := dropped.Swap(0); n > 0 {
					t.logger.Warn("packets dropped", "count", n)
				}
			}
		}
	}()

	// Step 7: UDP socket loop — reopens the socket on error until loopCtx is done.
	for {
		// Step 7a: Resolve peer address — retries each iteration to tolerate
		// DNS not being ready at startup (peer container may not exist yet).
		newPeer, resolveErr := net.ResolveUDPAddr("udp", t.config.PeerAddr)
		if resolveErr != nil {
			t.logger.Warn("peer address not yet resolvable, retrying",
				"addr", t.config.PeerAddr, "err", resolveErr)
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-loopCtx.Done():
				timer.Stop()
			case <-timer.C:
				continue
			}
			break
		}

		// Step 7b: Open UDP socket with SO_BROADCAST enabled.
		newConn, openErr := openUDPConn(listenAddr)
		if openErr != nil {
			t.logger.Error("failed to open UDP socket", "err", openErr)
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-loopCtx.Done():
				timer.Stop()
			case <-timer.C:
				continue
			}
			break
		}

		// Step 7c: Publish the new socket and peer so OnSend can use them.
		connMu.Lock()
		conn = newConn
		peerAddr = newPeer
		connMu.Unlock()

		// Step 7d: Read UDP datagrams and forward to rnsd via iface.Receive.
		// Accepts from all peers — no source-IP filter, matching UDPInterface.py.
		readErr := t.readLoop(loopCtx, newConn, iface)

		// Step 7e: Tear down current socket; decide whether to reopen.
		oldConn := newConn
		connMu.Lock()
		conn = nil
		connMu.Unlock()
		_ = oldConn.Close()

		if loopCtx.Err() != nil {
			break
		}
		t.logger.Warn("UDP socket error, reopening", "err", readErr)
	}

	// Step 8: Return the pipe interface result (nil on clean shutdown).
	return <-pipeDone
}

// openUDPConn creates a UDP socket bound to addr with SO_BROADCAST enabled.
func openUDPConn(addr *net.UDPAddr) (*net.UDPConn, error) {
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	// Enable SO_BROADCAST so we can send to broadcast addresses.
	// See: UDPInterface.py:process_outgoing — always enabled.
	rawConn, err := conn.SyscallConn()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	var setsockoptErr error
	if err := rawConn.Control(func(fd uintptr) {
		setsockoptErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if setsockoptErr != nil {
		_ = conn.Close()
		return nil, setsockoptErr
	}

	return conn, nil
}

// readLoop reads datagrams from conn and delivers them to iface.Receive.
// Returns when ctx is cancelled or conn produces an error.
func (t *Transport) readLoop(ctx context.Context, conn *net.UDPConn, iface *rnspipe.Interface) error {
	buf := make([]byte, hwMTU)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Use a short read deadline so we can check ctx.Done() regularly.
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}

		if err := iface.Receive(buf[:n]); err != nil {
			t.logger.Warn("iface.Receive error", "err", err)
		}
	}
}
