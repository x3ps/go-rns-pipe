---
title: Testing
weight: 2
description: "Running and writing tests for go-rns-pipe."
---

## Unit Tests

```bash
make test
# or
go test ./...
```

### Coverage by file

| File | Key coverage |
|------|-------------|
| `hdlc_test.go` | 11 known encode/decode vectors, byte stuffing, all 256 byte values round-trip, byte-at-a-time, oversized truncation, garbage/malformed/truncated frames, double-close, write-after-close, concurrent write/close, HWMTU boundary ±1, 200 random-payload round-trips |
| `config_test.go` | default values, negative clamping, MTU > HWMTU warning, custom logger |
| `pipe_test.go` | lifecycle (start/stop/restart), `ErrNoHandler` guard, concurrent `Receive`, `ErrOffline`, traffic counters (`SentPackets`, `ReceivedPackets`, `DroppedPackets`), goroutine leak prevention, `ExitOnEOF` |
| `reconnect_test.go` | fixed delay, zero delay on first attempt, exponential progression, jitter bounds, 60s cap |
| `integration_test.go` | `OnSend` error logging, dropped packet logging, drain-on-shutdown, `OnStatus` transitions, reconnect-with-new-stdin, concurrent inbound+outbound, full round-trips |
| `parity_test.go` | byte-exact round-trip via embedded Python decoder, FLAG/ESC byte handling, multi-frame echo, empty frame |

### Benchmarks

```bash
go test -bench=. -benchmem ./...
```

| Benchmark | What it measures |
|-----------|-----------------|
| `BenchmarkEncode` | HDLC frame encoding throughput (allocations per call) |
| `BenchmarkDecode` | HDLC stream decoding throughput |
| `BenchmarkRoundTrip` | Encode + Decode combined throughput |

See [Benchmarks](../benchmarks) for full details on running and interpreting results.

### Race detector

The `make test` target includes `-race`. To run it manually:

```bash
go test -race ./...
```

## Integration Tests

Integration tests live in `integration_test.go` and run with the standard `go test ./...` command — no build tag is required.

They use `io.Pipe` pairs instead of `os.Stdin`/`os.Stdout`, wired together with the `newTestPipe` helper:

```go
// newTestPipe creates an Interface wired to io.Pipe pairs.
// Returns the interface, stdin writer (inject data), stdout reader (read outbound frames).
func newTestPipe(t *testing.T, opts ...func(*Config)) (*Interface, *io.PipeWriter, *io.PipeReader)
```

A typical assertion waits on a channel with a timeout rather than reading synchronously:

```go
iface, stdinW, _ := newTestPipe(t)
iface.OnSend(func(pkt []byte) error {
    received <- pkt
    return nil
})

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
go iface.Start(ctx)

waitOnline(t, iface)

var enc Encoder
_, _ = stdinW.Write(enc.Encode([]byte("hello")))

select {
case pkt := <-received:
    if !bytes.Equal(pkt, []byte("hello")) {
        t.Errorf("got %q, want %q", pkt, "hello")
    }
case <-time.After(2 * time.Second):
    t.Fatal("timeout waiting for packet")
}
```

The `waitOnline` helper polls `iface.IsOnline()` with a 2-second deadline, ensuring the pipe is ready before injecting frames.

## Parity Tests

`parity_test.go` embeds a minimal Python HDLC decoder/encoder inline and uses `os/exec` to run it as a subprocess. Each test encodes a payload in Go, pipes it to the Python script, and verifies the decoded bytes match exactly.

Tests are skipped automatically when Python 3 or the `rns` package is not available:

```
--- SKIP: TestHDLCParityPython (Python not available)
```

Parity tests cover:
- Byte-exact round-trip for arbitrary payloads
- FLAG (`0x7E`) and ESC (`0x7D`) byte handling
- Multi-frame sequences
- Empty frame edge case

## E2E Tests

End-to-end tests use a real `rnsd` instance (Python RNS) and verify wire compatibility:

```bash
make e2e       # all E2E tests
make e2e-tcp   # TCP transport only
make e2e-udp   # UDP transport only
```

**Requirements:** Python 3.10+ with `rns` installed (`pip install rns`).

E2E test categories:
- Packet delivery: basic send/receive
- Channel ordering: packets delivered in order
- Large payloads: up to MTU-sized packets
- Link lifecycle: establish, use, tear down
- Parity and fidelity: byte-exact payload comparison
- Resource transfers: metadata and varied sizes
- Stress tests: many packets, many links

## Random Fuzzing

`TestEncodeDecodeRandomFuzzing` in `hdlc_test.go` runs 200 parallel round-trips with random payloads of random sizes (0–2048 bytes), using a fixed seed for reproducibility:

```bash
go test -run TestEncodeDecodeRandomFuzzing -v ./...
```

To extend coverage, add payloads that exercise specific byte patterns directly to `hdlc_test.go`.

## Writing New Tests

Use `newTestPipe` to create a test-wired interface, then wait on channels with timeouts:

```go
func TestMyFeature(t *testing.T) {
    t.Parallel()

    received := make(chan []byte, 1)
    iface, stdinW, _ := newTestPipe(t)
    iface.OnSend(func(pkt []byte) error {
        received <- append([]byte(nil), pkt...)
        return nil
    })

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go iface.Start(ctx)

    waitOnline(t, iface)

    var enc Encoder
    _, _ = stdinW.Write(enc.Encode([]byte("hello")))

    select {
    case pkt := <-received:
        if !bytes.Equal(pkt, []byte("hello")) {
            t.Errorf("got %q, want %q", pkt, "hello")
        }
    case <-time.After(2 * time.Second):
        t.Fatal("timeout waiting for packet")
    }
}
```

Key points:
- Always use `t.Parallel()` for independent tests
- Wait with a `select` + `time.After` timeout — never assume synchronous delivery
- Copy slices before storing (`append([]byte(nil), pkt...)`) if the callback reuses buffers
- Use `context.WithCancel` and `defer cancel()` to stop the interface cleanly
