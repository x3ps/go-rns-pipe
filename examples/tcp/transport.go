package main

import (
	"context"
	"io"
	"net"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// writeDeadline is the timeout for writing a single packet to a TCP connection.
// Prevents slow or stalled clients from blocking the sender.
const writeDeadline = 5 * time.Second

// writePacket HDLC-encodes a packet and writes it to a TCP connection.
// A write deadline is set to prevent slow clients from blocking indefinitely.
func writePacket(conn net.Conn, enc *rnspipe.Encoder, packet []byte) error {
	frame := enc.Encode(packet)
	if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	_, err := conn.Write(frame)
	return err
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
