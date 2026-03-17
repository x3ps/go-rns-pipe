package main

import (
	"context"
	"log/slog"
	"net"
	"sync"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// connPool manages a set of active TCP client connections. It supports
// concurrent broadcast (packets from rnsd to all clients) and safe removal
// of failed connections.
type connPool struct {
	mu    sync.RWMutex
	conns map[string]net.Conn
	enc   rnspipe.Encoder
	log   *slog.Logger
}

func newConnPool(logger *slog.Logger) *connPool {
	return &connPool{
		conns: make(map[string]net.Conn),
		log:   logger,
	}
}

func (p *connPool) add(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	p.mu.Lock()
	p.conns[addr] = conn
	p.mu.Unlock()
	p.log.Info("client connected", "addr", addr, "clients", p.count())
}

func (p *connPool) remove(addr string) {
	p.mu.Lock()
	if conn, ok := p.conns[addr]; ok {
		_ = conn.Close()
		delete(p.conns, addr)
	}
	p.mu.Unlock()
	p.log.Info("client disconnected", "addr", addr, "clients", p.count())
}

func (p *connPool) count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.conns)
}

// broadcast writes an HDLC-encoded packet to all connected clients.
// Failed writes cause the connection to be removed from the pool.
// See: TCPInterface.py — server spawns per-client handlers; since we have a
// single pipe to rnsd, outbound packets must be broadcast to all TCP clients.
func (p *connPool) broadcast(packet []byte) error {
	p.mu.RLock()
	// Snapshot connections under read lock.
	snapshot := make(map[string]net.Conn, len(p.conns))
	for addr, conn := range p.conns {
		snapshot[addr] = conn
	}
	p.mu.RUnlock()

	var failed []string
	for addr, conn := range snapshot {
		if err := writePacket(conn, &p.enc, packet); err != nil {
			p.log.Warn("broadcast write failed", "addr", addr, "error", err)
			failed = append(failed, addr)
		}
	}

	for _, addr := range failed {
		p.remove(addr)
	}
	return nil
}

func (p *connPool) closeAll() {
	p.mu.Lock()
	for addr, conn := range p.conns {
		_ = conn.Close()
		delete(p.conns, addr)
	}
	p.mu.Unlock()
}

// runServer listens for incoming TCP connections and bridges each client to the
// pipe interface. Packets from rnsd are broadcast to all connected clients.
// See: TCPInterface.py — TCPServerInterface accept loop
func runServer(ctx context.Context, cfg Config, iface *rnspipe.Interface, logger *slog.Logger) error {
	pool := newConnPool(logger)
	defer pool.closeAll()

	// Packets from rnsd (via pipe) are broadcast to all TCP clients.
	iface.OnSend(func(pkt []byte) error {
		return pool.broadcast(pkt)
	})

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	logger.Info("server listening", "addr", ln.Addr())

	// Close the listener when context is cancelled to unblock Accept.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warn("accept error", "error", err)
			continue
		}

		// See: TCPInterface.py — TCP_NODELAY enabled
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}

		pool.add(conn)
		go handleClient(ctx, conn, pool, cfg, iface, logger)
	}
}

// handleClient reads HDLC packets from a single TCP client and forwards them
// to rnsd via the pipe interface.
func handleClient(ctx context.Context, conn net.Conn, pool *connPool, cfg Config, iface *rnspipe.Interface, logger *slog.Logger) {
	addr := conn.RemoteAddr().String()
	defer pool.remove(addr)

	packets := make(chan []byte, 64)
	readDone := make(chan error, 1)
	readCtx, readCancel := context.WithCancel(ctx)
	defer readCancel()

	go func() {
		readDone <- readPackets(readCtx, conn, iface.HWMTU(), packets)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case pkt := <-packets:
			if err := iface.Receive(pkt); err != nil {
				logger.Warn("pipe receive error", "addr", addr, "error", err)
			}
		case err := <-readDone:
			if err != nil {
				logger.Debug("client read error", "addr", addr, "error", err)
			}
			return
		}
	}
}
