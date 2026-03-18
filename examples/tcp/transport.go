package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

const (
	// writeDeadline is the timeout for writing a single packet to a TCP connection.
	// Prevents slow or stalled clients from blocking the sender.
	writeDeadline = 5 * time.Second

	// tcpHWMTU is the maximum HDLC frame size accepted from a TCP peer.
	// Matches TCPInterface.py HW_MTU = 262144 so packets up to that size
	// are decoded correctly. Intentionally larger than the pipe-side HWMTU
	// (1064, PipeInterface.py), which governs rnsd ↔ rns-tcp-iface only.
	// See: RNS/Interfaces/TCPInterface.py — class TCPInterface: HW_MTU = 262144
	tcpHWMTU = 262144
)

// writePacket HDLC-encodes a packet and writes it to a TCP connection.
// A write deadline is set to prevent slow clients from blocking indefinitely.
func writePacket(conn net.Conn, enc *rnspipe.Encoder, packet []byte) error {
	frame := enc.Encode(packet)
	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	n, err := conn.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

// setTCPSocketOptions applies socket options matching TCPInterface.py:
// TCP_NODELAY, SO_KEEPALIVE, TCP_KEEPIDLE=5s (SetKeepAlivePeriod); on Linux
// also sets TCP_KEEPINTVL=2s, TCP_KEEPCNT=12, TCP_USER_TIMEOUT=24s —
// matching TCPInterface.py standard config.
func setTCPSocketOptions(conn *net.TCPConn, logger *slog.Logger) {
	if err := conn.SetNoDelay(true); err != nil {
		logger.Warn("TCP_NODELAY failed", "error", err)
	}
	if err := conn.SetKeepAlive(true); err != nil {
		logger.Warn("SO_KEEPALIVE failed", "error", err)
	}
	// Sets TCP_KEEPIDLE to 5s (TCPInterface.py: keepalive_idle=5).
	if err := conn.SetKeepAlivePeriod(5 * time.Second); err != nil {
		logger.Warn("TCP keep-alive period failed", "error", err)
	}
	setTCPPlatformOptions(conn, logger) // platform-specific
}

// readPackets reads from a TCP connection, HDLC-decodes the stream, and sends
// decoded packets to the provided channel. Returns when the connection closes
// or ctx is cancelled.
// See: TCPInterface.py — no app-level handshake, raw HDLC on connect
func readPackets(ctx context.Context, conn net.Conn, hwMTU int, packets chan<- []byte) error {
	decoder := rnspipe.NewDecoder(hwMTU, 64)

	// Feed TCP bytes into the HDLC decoder in a goroutine.
	// decoder.Close() is called inside the goroutine after io.Copy exits so
	// the packets channel is only closed once all bytes are processed.
	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(decoder, conn)
		decoder.Close()
		readErr <- err
	}()

	pktsC := decoder.Packets()
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close() // unblock io.Copy
			<-readErr        // wait for goroutine to exit
			return ctx.Err()
		case err := <-readErr:
			// Drain remaining decoded packets before returning.
			for {
				select {
				case pkt, ok := <-pktsC:
					if !ok {
						return err
					}
					select {
					case packets <- pkt:
					default:
						// receiver gone, drop remaining
						return err
					}
				default:
					return err
				}
			}
		case pkt, ok := <-pktsC:
			if !ok {
				// Channel closed; disable this case and wait for readErr.
				pktsC = nil
				continue
			}
			select {
			case packets <- pkt:
			case <-ctx.Done():
				_ = conn.Close()
				<-readErr
				return ctx.Err()
			}
		}
	}
}
