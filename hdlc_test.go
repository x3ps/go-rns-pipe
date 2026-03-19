package rnspipe

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

// Known vectors computed from the Python RNS HDLC.escape() algorithm:
//  1. Replace 0x7D → 0x7D 0x5D
//  2. Replace 0x7E → 0x7D 0x5E
//  3. Wrap: 0x7E + escaped + 0x7E
var hdlcVectors = []struct {
	name  string
	input []byte
	frame []byte
}{
	{"V1_simple_byte", []byte{0x41}, []byte{0x7E, 0x41, 0x7E}},
	{"V2_flag_byte", []byte{0x7E}, []byte{0x7E, 0x7D, 0x5E, 0x7E}},
	{"V3_escape_byte", []byte{0x7D}, []byte{0x7E, 0x7D, 0x5D, 0x7E}},
	{"V4_esc_then_flag", []byte{0x7D, 0x7E}, []byte{0x7E, 0x7D, 0x5D, 0x7D, 0x5E, 0x7E}},
	{"V5_flag_then_esc", []byte{0x7E, 0x7D}, []byte{0x7E, 0x7D, 0x5E, 0x7D, 0x5D, 0x7E}},
	{"V6_three_flags", []byte{0x7E, 0x7E, 0x7E}, []byte{0x7E, 0x7D, 0x5E, 0x7D, 0x5E, 0x7D, 0x5E, 0x7E}},
	{"V7_three_escapes", []byte{0x7D, 0x7D, 0x7D}, []byte{0x7E, 0x7D, 0x5D, 0x7D, 0x5D, 0x7D, 0x5D, 0x7E}},
	{"V8_null_byte", []byte{0x00}, []byte{0x7E, 0x00, 0x7E}},
	{"V9_0xFF", []byte{0xFF}, []byte{0x7E, 0xFF, 0x7E}},
	{"V10_0x5D_passthrough", []byte{0x5D}, []byte{0x7E, 0x5D, 0x7E}},
	{"V11_0x5E_passthrough", []byte{0x5E}, []byte{0x7E, 0x5E, 0x7E}},
}

func TestEncodeKnownVectors(t *testing.T) {
	t.Parallel()
	enc := &Encoder{}
	for _, v := range hdlcVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			got := enc.Encode(v.input)
			if !bytes.Equal(got, v.frame) {
				t.Errorf("Encode(%x) = %x, want %x", v.input, got, v.frame)
			}
		})
	}
}

func TestDecodeKnownVectors(t *testing.T) {
	t.Parallel()
	for _, v := range hdlcVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			dec := NewDecoder(1064, 1)
			defer dec.Close()
			if _, err := dec.Write(v.frame); err != nil {
				t.Fatalf("Write: %v", err)
			}
			select {
			case pkt := <-dec.Packets():
				if !bytes.Equal(pkt, v.input) {
					t.Errorf("Decode(%x) = %x, want %x", v.frame, pkt, v.input)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for packet")
			}
		})
	}
}

func TestEncodeDecodeAllBytes(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}

	enc := &Encoder{}
	frame := enc.Encode(payload)

	dec := NewDecoder(2048, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, payload) {
			t.Errorf("round-trip failed: got %d bytes, want 256", len(pkt))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDecoderByteAtATime(t *testing.T) {
	t.Parallel()

	payload := []byte("byte-at-a-time")
	enc := &Encoder{}
	frame := enc.Encode(payload)

	dec := NewDecoder(1064, 1)
	defer dec.Close()

	for i, b := range frame {
		if _, err := dec.Write([]byte{b}); err != nil {
			t.Fatalf("Write byte %d: %v", i, err)
		}
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, payload) {
			t.Errorf("got %x, want %x", pkt, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDecoderMultipleFrames(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	payloads := [][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}

	var buf bytes.Buffer
	for _, p := range payloads {
		buf.Write(enc.Encode(p))
	}

	dec := NewDecoder(1064, 8)
	defer dec.Close()
	if _, err := dec.Write(buf.Bytes()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	for i, want := range payloads {
		select {
		case pkt := <-dec.Packets():
			if !bytes.Equal(pkt, want) {
				t.Errorf("frame %d: got %x, want %x", i, pkt, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for frame %d", i)
		}
	}
}

func TestDecoderTruncatesOversize(t *testing.T) {
	t.Parallel()

	const hwMTU = 10
	payload := make([]byte, 20)
	for i := range payload {
		payload[i] = byte(i + 1)
	}

	enc := &Encoder{}
	frame := enc.Encode(payload)

	dec := NewDecoder(hwMTU, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case pkt := <-dec.Packets():
		if len(pkt) > hwMTU {
			t.Errorf("packet length %d exceeds hwMTU %d", len(pkt), hwMTU)
		}
		if len(pkt) != hwMTU {
			t.Errorf("expected truncated packet of %d bytes, got %d", hwMTU, len(pkt))
		}
		// First hwMTU bytes should match.
		if !bytes.Equal(pkt, payload[:hwMTU]) {
			t.Errorf("got %x, want %x", pkt, payload[:hwMTU])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDecoderGarbageBeforeFrame(t *testing.T) {
	t.Parallel()

	payload := []byte("clean")
	enc := &Encoder{}
	frame := enc.Encode(payload)

	// Prepend garbage that doesn't include FLAG.
	garbage := []byte{0x01, 0x02, 0x03, 0xFF, 0xAA}
	data := append(garbage, frame...)

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, payload) {
			t.Errorf("got %x, want %x", pkt, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDecoderMalformedEscape(t *testing.T) {
	t.Parallel()

	// Frame with ESC followed by 0x42 (not a valid escape target).
	// Decoder should pass 0x42 through unchanged.
	frame := []byte{HDLCFlag, HDLCEscape, 0x42, HDLCFlag}

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write(frame); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case pkt := <-dec.Packets():
		if !bytes.Equal(pkt, []byte{0x42}) {
			t.Errorf("got %x, want [42]", pkt)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDecoderTruncatedFrameNoOutput(t *testing.T) {
	t.Parallel()

	dec := NewDecoder(1064, 1)
	defer dec.Close()
	if _, err := dec.Write([]byte{HDLCFlag, 0x01, 0x02}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case pkt := <-dec.Packets():
		t.Fatalf("unexpected packet from truncated frame: %x", pkt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDecoderMixedCorruptTruncatedEmpty(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	validFrame := enc.Encode([]byte{0x01, 0x02, 0x03, 0x04})

	corruptedFrame := append([]byte(nil), validFrame...)
	corruptedFrame[2] ^= 0xFF

	truncatedFrame := []byte{HDLCFlag, 0x01, 0x02}
	emptyFrame := []byte{HDLCFlag, HDLCFlag}
	stream := append(append(append(corruptedFrame, truncatedFrame...), emptyFrame...), validFrame...)

	dec := NewDecoder(1064, 4)
	defer dec.Close()
	if _, err := dec.Write(stream); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := [][]byte{
		{0x01, 0xFD, 0x03, 0x04},
		{0x01, 0x02},
		{},
	}

	for i, expected := range want {
		select {
		case pkt := <-dec.Packets():
			if !bytes.Equal(pkt, expected) {
				t.Fatalf("packet %d: got %x, want %x", i, pkt, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for packet %d", i)
		}
	}

	select {
	case pkt := <-dec.Packets():
		t.Fatalf("unexpected extra packet: %x", pkt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDecoderDoubleClose(t *testing.T) {
	t.Parallel()

	dec := NewDecoder(1064, 1)
	dec.Close()
	dec.Close() // must not panic
}

func TestDecoderWriteAfterClose(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	dec := NewDecoder(1064, 1)
	dec.Close()

	frame := enc.Encode([]byte("after-close"))
	_, err := dec.Write(frame)
	if err != io.ErrClosedPipe {
		t.Fatalf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestDecoderConcurrentWriteClose(t *testing.T) {
	t.Parallel()

	const goroutines = 50
	enc := &Encoder{}
	frame := enc.Encode([]byte("concurrent"))

	dec := NewDecoder(1064, 64)

	var wg sync.WaitGroup

	// Goroutines writing concurrently.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := dec.Write(frame)
			if err != nil && err != io.ErrClosedPipe {
				panic(fmt.Sprintf("unexpected Write error: %v", err))
			}
		}()
	}

	// One goroutine closing concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		dec.Close()
	}()

	wg.Wait()

	// After Close, Write must return io.ErrClosedPipe.
	_, err := dec.Write(frame)
	if err != io.ErrClosedPipe {
		t.Fatalf("expected io.ErrClosedPipe after Close, got %v", err)
	}
}

func TestEncodeDecodeRandomFuzzing(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	rng := rand.New(rand.NewPCG(42, 0))

	for i := range 200 {
		size := rng.IntN(2049) // [0, 2048]
		payload := make([]byte, size)
		for j := range payload {
			payload[j] = byte(rng.IntN(256))
		}

		t.Run(fmt.Sprintf("iter_%d_size_%d", i, size), func(t *testing.T) {
			t.Parallel()

			frame := enc.Encode(payload)
			dec := NewDecoder(2048, 1)
			defer dec.Close()

			if _, err := dec.Write(frame); err != nil {
				t.Fatalf("Write: %v", err)
			}

			select {
			case pkt := <-dec.Packets():
				if !bytes.Equal(pkt, payload) {
					t.Errorf("round-trip mismatch: got %d bytes, want %d", len(pkt), len(payload))
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for packet")
			}
		})
	}
}

func TestDecoderHWMTUBoundaryExact(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	const hwMTU = 1064

	cases := []struct {
		name      string
		size      int
		wantSize  int
		truncated bool
	}{
		{"at_minus_one", 1063, 1063, false},
		{"at_exact", 1064, 1064, false},
		{"at_plus_one", 1065, 1064, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := make([]byte, tc.size)
			for i := range payload {
				payload[i] = byte(i % 251) // avoid 0x7E/0x7D patterns for simplicity
			}

			frame := enc.Encode(payload)
			dec := NewDecoder(hwMTU, 1)
			defer dec.Close()

			if _, err := dec.Write(frame); err != nil {
				t.Fatalf("Write: %v", err)
			}

			select {
			case pkt := <-dec.Packets():
				if len(pkt) != tc.wantSize {
					t.Errorf("got %d bytes, want %d", len(pkt), tc.wantSize)
				}
				if !bytes.Equal(pkt, payload[:tc.wantSize]) {
					t.Errorf("first %d bytes do not match", tc.wantSize)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for packet")
			}
		})
	}
}

func BenchmarkEncode(b *testing.B) {
	enc := &Encoder{}
	payload := make([]byte, 500) // typical RNS MTU
	for i := range payload {
		payload[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encode(payload)
	}
}

func BenchmarkDecode(b *testing.B) {
	enc := &Encoder{}
	payload := make([]byte, 500)
	for i := range payload {
		payload[i] = byte(i)
	}
	frame := enc.Encode(payload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec := NewDecoder(1064, 1)
		_, _ = dec.Write(frame)
		<-dec.Packets()
		dec.Close()
	}
}

func BenchmarkRoundTrip(b *testing.B) {
	enc := &Encoder{}
	payload := make([]byte, 500)
	for i := range payload {
		payload[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame := enc.Encode(payload)
		dec := NewDecoder(1064, 1)
		_, _ = dec.Write(frame)
		<-dec.Packets()
		dec.Close()
	}
}

func TestDecoderDropMultiple(t *testing.T) {
	t.Parallel()

	enc := &Encoder{}
	dec := NewDecoder(1064, 1) // channel capacity 1
	defer dec.Close()

	// Write 5 frames without consuming. First fills the channel, rest are dropped.
	for i := range 5 {
		frame := enc.Encode([]byte(fmt.Sprintf("pkt%d", i)))
		if _, err := dec.Write(frame); err != nil {
			t.Fatalf("Write frame %d: %v", i, err)
		}
	}

	if got := dec.DroppedPackets(); got != 4 {
		t.Errorf("DroppedPackets() = %d, want 4", got)
	}
}
