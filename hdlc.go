package rnspipe

import (
	"sync"
	"sync/atomic"
)

// HDLC constants for simplified HDLC framing, matching the Python RNS
// implementation. The framing is similar to PPP.
// See: PipeInterface.py#L40-L42 — HDLC class constants
const (
	HDLCFlag    = 0x7E // Frame delimiter
	HDLCEscape  = 0x7D // Escape character
	HDLCEscMask = 0x20 // XOR mask applied to escaped bytes
)

// Encoder wraps raw packets into HDLC frames.
type Encoder struct{}

// Encode wraps a packet in HDLC framing: FLAG + escaped(data) + FLAG.
// The escape order is critical: first escape ESC bytes, then FLAG bytes.
// See: PipeInterface.py#L44-L47 — HDLC.escape static method
// See: PipeInterface.py#L103-L104 — process_outgoing framing
func (e *Encoder) Encode(packet []byte) []byte {
	// Pre-allocate with some headroom for escapes
	out := make([]byte, 0, len(packet)+len(packet)/4+2)
	out = append(out, HDLCFlag)

	// Escape order matters: ESC first, then FLAG.
	// See: PipeInterface.py#L45 — escape ESC before FLAG
	for _, b := range packet {
		switch b {
		case HDLCEscape:
			out = append(out, HDLCEscape, HDLCEscape^HDLCEscMask)
		case HDLCFlag:
			out = append(out, HDLCEscape, HDLCFlag^HDLCEscMask)
		default:
			out = append(out, b)
		}
	}

	out = append(out, HDLCFlag)
	return out
}

// Decoder reads a byte stream and extracts complete HDLC-framed packets.
// It implements io.Writer so it can be fed raw bytes incrementally.
type Decoder struct {
	inFrame   bool
	escape    bool
	buf       []byte
	hwMTU     int
	packets   chan []byte
	closeOnce sync.Once
	dropped   atomic.Uint64
}

// NewDecoder creates a Decoder with the given hardware MTU limit and packet
// channel capacity.
// See: PipeInterface.py#L113 — buffer limited to self.HW_MTU
func NewDecoder(hwMTU, chanSize int) *Decoder {
	return &Decoder{
		hwMTU:   hwMTU,
		packets: make(chan []byte, chanSize),
	}
}

// Write feeds raw bytes into the decoder. Complete packets are emitted on the
// Packets channel. The decoding logic exactly mirrors PipeInterface.readLoop.
// See: PipeInterface.py#L110-L134 — readLoop byte-by-byte state machine
func (d *Decoder) Write(b []byte) (int, error) {
	for _, byte_ := range b {
		if d.inFrame && byte_ == HDLCFlag {
			// End of frame — deliver the packet (even if empty, matching Python upstream
			// which calls process_incoming(data_buffer) unconditionally).
			// See: PipeInterface.py#L121-L123
			d.escape = false
			d.inFrame = false
			pkt := make([]byte, len(d.buf))
			copy(pkt, d.buf)
			select {
			case d.packets <- pkt:
			default:
				// Channel full — drop packet
				d.dropped.Add(1)
			}
			d.buf = d.buf[:0]
		} else if byte_ == HDLCFlag {
			// Start of frame
			// See: PipeInterface.py#L124-L126
			d.inFrame = true
			d.escape = false
			d.buf = d.buf[:0]
		} else if d.inFrame && len(d.buf) < d.hwMTU {
			if byte_ == HDLCEscape {
				// Next byte is escaped
				// See: PipeInterface.py#L128-L129
				d.escape = true
			} else {
				if d.escape {
					// Conditional unescaping exactly mirrors Python PipeInterface.py readLoop.
					// Only the two valid escape sequences are remapped; any other byte following
					// ESC is passed through unchanged (matching Python behaviour for malformed data).
					// See: PipeInterface.py#L131-L134
					switch byte_ {
					case HDLCFlag ^ HDLCEscMask: // 0x5E → 0x7E
						byte_ = HDLCFlag
					case HDLCEscape ^ HDLCEscMask: // 0x5D → 0x7D
						byte_ = HDLCEscape
					}
					d.escape = false
				}
				d.buf = append(d.buf, byte_)
			}
		}
	}
	return len(b), nil
}

// Packets returns a read-only channel that emits decoded packets.
func (d *Decoder) Packets() <-chan []byte {
	return d.packets
}

// Close closes the packets channel. Safe to call multiple times.
func (d *Decoder) Close() {
	d.closeOnce.Do(func() { close(d.packets) })
}

// DroppedPackets returns the number of packets dropped due to a full channel.
func (d *Decoder) DroppedPackets() uint64 {
	return d.dropped.Load()
}
