package main

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// clientConn holds the active TCP connection with mutex protection, allowing
// safe concurrent access from the OnSend callback during reconnection.
type clientConn struct {
	mu   sync.Mutex
	conn net.Conn
	enc  rnspipe.Encoder
}

// send HDLC-encodes and writes a packet to the active connection.
// Returns ErrOffline if no connection is active.
func (c *clientConn) send(packet []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return rnspipe.ErrOffline
	}
	return writePacket(c.conn, &c.enc, packet)
}

// setConn replaces the active connection.
func (c *clientConn) setConn(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}

// close closes the active connection if any.
func (c *clientConn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// runClient connects to cfg.PeerAddr and bridges TCP traffic to the pipe
// interface. It reconnects with exponential backoff on disconnection.
// See: TCPInterface.py — TCPClientInterface connect/reconnect logic
func runClient(ctx context.Context, cfg Config, iface *rnspipe.Interface, logger *slog.Logger, ready chan struct{}) error {
	cc := &clientConn{}
	defer cc.close()

	// Register OnSend once — it persists across reconnections.
	// Packets from rnsd (via pipe) are forwarded to the TCP peer.
	iface.OnSend(func(pkt []byte) error {
		return cc.send(pkt)
	})
	close(ready) // signal: OnSend is registered, safe to start reading stdin

	attempt := 0
	for {
		logger.Info("connecting to peer", "addr", cfg.PeerAddr, "attempt", attempt+1)

		conn, err := dial(ctx, cfg.PeerAddr)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warn("connection failed", "addr", cfg.PeerAddr, "error", err)
			attempt++
			if err := sleepBackoff(ctx, cfg.ReconnectDelay, attempt); err != nil {
				return err
			}
			continue
		}

		// See: TCPInterface.py — TCP_NODELAY, SO_KEEPALIVE, TCP_USER_TIMEOUT
		if tc, ok := conn.(*net.TCPConn); ok {
			setTCPSocketOptions(tc, logger)
		}

		logger.Info("connected to peer", "addr", cfg.PeerAddr)
		cc.setConn(conn)
		attempt = 0

		// Read TCP packets and forward to rnsd via pipe.
		packets := make(chan []byte, 64)
		readCtx, readCancel := context.WithCancel(ctx)
		readDone := make(chan error, 1)
		go func() {
			readDone <- readPackets(readCtx, conn, tcpHWMTU, packets)
		}()

	loop:
		for {
			select {
			case <-ctx.Done():
				readCancel()
				<-readDone
				return ctx.Err()
			case pkt := <-packets:
				// Packets from TCP peer → pipe → rnsd
				if err := iface.Receive(pkt); err != nil {
					logger.Warn("pipe receive error", "error", err)
				}
			case err := <-readDone:
				// TCP connection closed or errored.
				// See: TCPInterface.py — empty recv() = peer closed
				if err != nil {
					logger.Warn("peer disconnected", "addr", cfg.PeerAddr, "error", err)
				} else {
					logger.Info("peer closed connection", "addr", cfg.PeerAddr)
				}
				readCancel()
				cc.close()
				attempt++
				break loop
			}
		}

		logger.Info("reconnecting", "delay", backoff(cfg.ReconnectDelay, attempt))
		if err := sleepBackoff(ctx, cfg.ReconnectDelay, attempt); err != nil {
			return err
		}
	}
}

// dial connects to addr with context cancellation support.
func dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// sleepBackoff waits for an exponential backoff duration with jitter.
func sleepBackoff(ctx context.Context, base time.Duration, attempt int) error {
	delay := backoff(base, attempt)
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoff returns 0 for the first attempt and base for all subsequent attempts.
// Fixed delay matching TCPInterface.py RECONNECT_WAIT = 5.
func backoff(base time.Duration, attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}
	return base
}
