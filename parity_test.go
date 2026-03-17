//go:build integration

package rnspipe_test

// Parity tests against the Python Reticulum reference implementation.
//
// Requires: python3 in PATH with Reticulum installed.
// Run: go test -tags=integration -count=1 ./...

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

// pyHDLCScript is a minimal Python snippet that reads one HDLC frame from
// stdin using RNS framing and writes back the decoded payload.
const pyHDLCScript = `
import sys, struct

FLAG  = 0x7E
ESC   = 0x7D
ESC_M = 0x20

def hdlc_decode(data):
    out = bytearray()
    escaping = False
    for b in data:
        if b == FLAG:
            if out:
                return bytes(out)
        elif b == ESC:
            escaping = True
        elif escaping:
            out.append(b ^ ESC_M)
            escaping = False
        else:
            out.append(b)
    return None

buf = sys.stdin.buffer.read(4096)
payload = hdlc_decode(buf)
if payload is not None:
    sys.stdout.buffer.write(payload)
    sys.stdout.buffer.flush()
`

func TestHDLCParityPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	// Write the Python script to a temp file.
	tmp, err := os.CreateTemp("", "hdlc_parity_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyHDLCScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	payload := []byte("hello-parity-test")
	enc := &rnspipe.Encoder{}
	frame := enc.Encode(payload)

	cmd := exec.Command(python, tmp.Name())
	cmd.Stdin = bytes.NewReader(frame)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("python script error: %v", err)
	}

	if !bytes.Equal(out, payload) {
		t.Errorf("Python decoded %q, want %q", out, payload)
	}
}

// TestHDLCParityBinaryPayload verifies that payloads containing FLAG (0x7E)
// and ESC (0x7D) bytes are encoded by Go and correctly decoded by Python.
func TestHDLCParityBinaryPayload(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	tmp, err := os.CreateTemp("", "hdlc_parity_binary_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyHDLCScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	vectors := []struct {
		name    string
		payload []byte
	}{
		{"flag_byte", []byte{0x7E}},
		{"esc_byte", []byte{0x7D}},
		{"flag_and_esc", []byte{0x7E, 0x7D}},
		{"esc_and_flag", []byte{0x7D, 0x7E}},
		{"mixed_binary", []byte{0x01, 0x7E, 0x02, 0x7D, 0x03}},
		{"three_flags", []byte{0x7E, 0x7E, 0x7E}},
		{"three_escapes", []byte{0x7D, 0x7D, 0x7D}},
	}

	enc := &rnspipe.Encoder{}
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			frame := enc.Encode(v.payload)

			cmd := exec.Command(python, tmp.Name())
			cmd.Stdin = bytes.NewReader(frame)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("python script error: %v", err)
			}

			if !bytes.Equal(out, v.payload) {
				t.Errorf("Python decoded %x, want %x", out, v.payload)
			}
		})
	}
}

// pyEchoScript is a Python HDLC echo server: reads frames from stdin byte-by-byte,
// decodes them with correct two-state unescape logic, re-encodes and writes to stdout.
// The previous version had a bug where ESC handling used `continue` without XOR-ing
// the following byte — this version uses a proper in_esc state flag.
const pyEchoScript = `
import sys

FLAG  = 0x7E
ESC   = 0x7D
ESC_M = 0x20

def encode(data):
    out = bytearray([FLAG])
    for b in data:
        if b == ESC:
            out += bytes([ESC, ESC ^ ESC_M])
        elif b == FLAG:
            out += bytes([ESC, FLAG ^ ESC_M])
        else:
            out.append(b)
    out.append(FLAG)
    return bytes(out)

buf = bytearray()
while True:
    chunk = sys.stdin.buffer.read(1)
    if not chunk:
        break
    b = chunk[0]
    buf.append(b)
    while len(buf) >= 2:
        try:
            start = buf.index(FLAG)
        except ValueError:
            buf = bytearray()
            break
        try:
            end = buf.index(FLAG, start + 1)
        except ValueError:
            if start > 0:
                buf = buf[start:]
            break
        frame = buf[start+1:end]
        buf = buf[end+1:]
        payload = bytearray()
        in_esc = False
        for byte in frame:
            if in_esc:
                payload.append(byte ^ ESC_M)
                in_esc = False
            elif byte == ESC:
                in_esc = True
            else:
                payload.append(byte)
        sys.stdout.buffer.write(encode(bytes(payload)))
        sys.stdout.buffer.flush()
`

func runEchoTest(t *testing.T, payload []byte) []byte {
	t.Helper()

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	tmp, err := os.CreateTemp("", "pipe_parity_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyEchoScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	enc := &rnspipe.Encoder{}
	frame := enc.Encode(payload)

	cmd := exec.Command(python, tmp.Name())
	cmd.Stdin = bytes.NewReader(frame)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd start: %v", err)
	}
	defer func() { _ = cmd.Wait() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	iface := rnspipe.New(rnspipe.Config{
		Stdin:          stdoutPipe,
		Stdout:         io.Discard,
		ReconnectDelay: 10 * time.Millisecond,
	})

	var decoded []byte
	gotPacket := make(chan struct{}, 1)
	iface.OnSend(func(pkt []byte) error {
		decoded = append([]byte(nil), pkt...)
		select {
		case gotPacket <- struct{}{}:
		default:
		}
		return nil
	})

	go func() { _ = iface.Start(ctx) }()

	select {
	case <-gotPacket:
	case <-ctx.Done():
		t.Fatal("timeout waiting for decoded packet from Python echo server")
	}

	return decoded
}

func TestPipeInterfaceParityPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	// Verify Reticulum is importable (optional — skip if not installed).
	check := exec.Command(python, "-c", "import RNS")
	if err := check.Run(); err != nil {
		t.Skip("Reticulum not installed: python3 -c 'import RNS' failed")
	}

	tmp, err := os.CreateTemp("", "pipe_parity_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyEchoScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	enc := &rnspipe.Encoder{}
	payload := []byte("parity-roundtrip")
	frame := enc.Encode(payload)

	cmd := exec.Command(python, tmp.Name())
	cmd.Stdin = bytes.NewReader(frame)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd start: %v", err)
	}
	defer func() { _ = cmd.Wait() }()

	// Feed the Python echo server's output back through our Go decoder.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	iface := rnspipe.New(rnspipe.Config{
		Stdin:          stdoutPipe,
		Stdout:         io.Discard,
		ReconnectDelay: 10 * time.Millisecond,
	})

	var decoded []byte
	gotPacket := make(chan struct{}, 1)
	iface.OnSend(func(pkt []byte) error {
		decoded = append([]byte(nil), pkt...)
		select {
		case gotPacket <- struct{}{}:
		default:
		}
		return nil
	})

	go func() { _ = iface.Start(ctx) }()

	select {
	case <-gotPacket:
	case <-ctx.Done():
		t.Fatal("timeout waiting for decoded packet from Python echo server")
	}

	if !bytes.Equal(decoded, payload) {
		t.Errorf("round-trip via Python: got %q, want %q", decoded, payload)
	}
}

// TestPipeInterfaceParityEscapeRoundtrip verifies that payloads containing
// FLAG (0x7E) and ESC (0x7D) bytes survive a full encode→Python-decode→
// Python-encode→Go-decode round-trip with correct byte values.
func TestPipeInterfaceParityEscapeRoundtrip(t *testing.T) {
	vectors := []struct {
		name    string
		payload []byte
	}{
		{"flag_byte", []byte{0x7E}},
		{"esc_byte", []byte{0x7D}},
		{"flag_and_esc", []byte{0x7D, 0x7E}},
		{"mixed_binary", []byte{0x01, 0x7E, 0x02, 0x7D, 0x03}},
		{"three_flags", []byte{0x7E, 0x7E, 0x7E}},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			got := runEchoTest(t, v.payload)
			if !bytes.Equal(got, v.payload) {
				t.Errorf("round-trip: got %x, want %x", got, v.payload)
			}
		})
	}
}

// TestPipeInterfaceParityMultiFrame sends three frames to the Python echo server
// and verifies all three are echoed back in order.
func TestPipeInterfaceParityMultiFrame(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	tmp, err := os.CreateTemp("", "pipe_multi_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyEchoScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	payloads := [][]byte{
		[]byte("frame-one"),
		[]byte("frame-two"),
		[]byte("frame-three"),
	}

	enc := &rnspipe.Encoder{}
	var allFrames []byte
	for _, p := range payloads {
		allFrames = append(allFrames, enc.Encode(p)...)
	}

	cmd := exec.Command(python, tmp.Name())
	cmd.Stdin = bytes.NewReader(allFrames)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd start: %v", err)
	}
	defer func() { _ = cmd.Wait() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	iface := rnspipe.New(rnspipe.Config{
		Stdin:          stdoutPipe,
		Stdout:         io.Discard,
		ReconnectDelay: 10 * time.Millisecond,
	})

	var received [][]byte
	mu := make(chan struct{}, 1)
	allReceived := make(chan struct{}, 1)

	iface.OnSend(func(pkt []byte) error {
		mu <- struct{}{}
		received = append(received, append([]byte(nil), pkt...))
		if len(received) == len(payloads) {
			select {
			case allReceived <- struct{}{}:
			default:
			}
		}
		<-mu
		return nil
	})

	go func() { _ = iface.Start(ctx) }()

	select {
	case <-allReceived:
	case <-ctx.Done():
		t.Fatalf("timeout: got %d/%d frames", len(received), len(payloads))
	}

	for i, want := range payloads {
		if i >= len(received) {
			t.Fatalf("frame %d: not received", i)
		}
		if !bytes.Equal(received[i], want) {
			t.Errorf("frame %d: got %q, want %q", i, received[i], want)
		}
	}
}

// TestPipeInterfaceParityEmptyFrame sends an empty HDLC frame (FLAG+FLAG) to
// the Python echo server and verifies that the Go decoder delivers an empty
// packet, matching Python's process_incoming(b"") behavior.
func TestPipeInterfaceParityEmptyFrame(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not in PATH")
	}

	tmp, err := os.CreateTemp("", "pipe_empty_*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(pyEchoScript); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	_ = tmp.Close()

	// FLAG+FLAG is the encoding of an empty payload.
	emptyFrame := []byte{0x7E, 0x7E}

	cmd := exec.Command(python, tmp.Name())
	cmd.Stdin = bytes.NewReader(emptyFrame)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd start: %v", err)
	}
	defer func() { _ = cmd.Wait() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	iface := rnspipe.New(rnspipe.Config{
		Stdin:          stdoutPipe,
		Stdout:         io.Discard,
		ReconnectDelay: 10 * time.Millisecond,
	})

	gotPacket := make(chan []byte, 1)
	iface.OnSend(func(pkt []byte) error {
		select {
		case gotPacket <- append([]byte(nil), pkt...):
		default:
		}
		return nil
	})

	go func() { _ = iface.Start(ctx) }()

	select {
	case pkt := <-gotPacket:
		if len(pkt) != 0 {
			t.Errorf("expected empty packet, got %x", pkt)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for empty packet from Python echo server")
	}
}
