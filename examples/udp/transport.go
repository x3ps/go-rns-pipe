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
	config Config
	logger *slog.Logger
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
// Step 4: Start drop-counter logger.
// Step 5: Start pipe interface goroutine.
// Step 6: Register OnSend callback.
// Step 7: UDP socket loop (reconnects on socket error).
// Step 8: Return pipe interface error.
func (t *Transport) Start(ctx context.Context) error {
	// Step 1: Resolve ListenAddr and PeerAddr — fail fast on bad config.
	listenAddr, err := net.ResolveUDPAddr("udp", t.config.ListenAddr)
	if err != nil {
		return err
	}
	peerAddr, err := net.ResolveUDPAddr("udp", t.config.PeerAddr)
	if err != nil {
		return err
	}

	// Step 2: Create rnspipe.Interface connected to rnsd via stdin/stdout.
	iface := rnspipe.New(rnspipe.Config{
		Name:     t.config.Name,
		MTU:      t.config.MTU,
		LogLevel: t.config.LogLevel,
	})

	// Step 3: Register status callback — log online/offline transitions.
	iface.OnStatus(func(online bool) {
		if online {
			t.logger.Info("pipe interface online")
		} else {
			t.logger.Warn("pipe interface offline")
		}
	})

	// Step 4: Start drop-counter logger goroutine.
	// Oversized or mid-reconnect drops are counted and logged every 30s.
	var dropped atomic.Int64
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n := dropped.Swap(0); n > 0 {
					t.logger.Warn("packets dropped", "count", n)
				}
			}
		}
	}()

	// Step 5: Start pipe interface in background; capture result via channel.
	ifaceErr := make(chan error, 1)
	go func() { ifaceErr <- iface.Start(ctx) }()

	// conn and connMu protect access to the active UDP socket across the
	// OnSend callback and the socket loop below.
	var (
		connMu sync.RWMutex
		conn   *net.UDPConn
	)

	// Step 6: Register OnSend — forward pipe packets to UDP peer.
	// The callback closes over connMu/conn so it always uses the current socket.
	// See: UDPInterface.py:process_outgoing
	iface.OnSend(func(pkt []byte) error {
		connMu.RLock()
		c := conn
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
		_, err := c.WriteTo(pkt, peerAddr)
		return err
	})

	// Step 7: UDP socket loop — reopens the socket on error until ctx is done.
	for {
		// Step 7a: Open UDP socket with SO_BROADCAST enabled.
		newConn, openErr := openUDPConn(listenAddr)
		if openErr != nil {
			t.logger.Error("failed to open UDP socket", "err", openErr)
			select {
			case <-ctx.Done():
				break
			case <-time.After(100 * time.Millisecond):
				continue
			}
			break
		}

		// Step 7b: Publish the new socket so OnSend can use it.
		connMu.Lock()
		conn = newConn
		connMu.Unlock()

		// Step 7c: Read UDP datagrams and forward to rnsd via iface.Receive.
		// Accepts from all peers — no source-IP filter, matching UDPInterface.py.
		readErr := t.readLoop(ctx, newConn, iface)

		// Step 7d: Tear down current socket; decide whether to reopen.
		oldConn := newConn
		connMu.Lock()
		conn = nil
		connMu.Unlock()
		_ = oldConn.Close()

		if ctx.Err() != nil {
			break
		}
		t.logger.Warn("UDP socket error, reopening", "err", readErr)
	}

	// Step 8: Return the pipe interface result (nil on clean shutdown).
	return <-ifaceErr
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
