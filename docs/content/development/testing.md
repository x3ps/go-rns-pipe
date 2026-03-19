---
title: Testing
weight: 2
---

## Unit Tests

```bash
make test
# or
go test ./...
```

Covers:
- HDLC encoding/decoding correctness (`hdlc_test.go`)
- Framing edge cases: empty frames, escaping, oversized frames
- Thread-safety: concurrent `Write`/`Close` on `Decoder`
- Fuzzing: `FuzzHDLC` for malformed input
- Pipe lifecycle: start/stop, reconnect, concurrent `Receive` calls
- Config defaults and `ErrNoHandler` guard

## Integration Tests

```bash
go test ./... -tags integration
```

Tests the full pipe stack with `io.Pipe` pairs instead of `os.Stdin`/`os.Stdout`.

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

## Fuzzing

```bash
go test -fuzz=FuzzHDLC -fuzztime=30s ./...
```

The fuzz corpus covers:
- Empty input
- Single-byte sequences
- All special bytes (`0x7E`, `0x7D`)
- Valid and malformed frames

## Writing New Tests

For unit tests, use `io.Pipe` to inject test data:

```go
func TestMyFeature(t *testing.T) {
    stdinR, stdinW := io.Pipe()
    stdoutR, stdoutW := io.Pipe()

    iface := rnspipe.New(rnspipe.Config{
        Stdin:  stdinR,
        Stdout: stdoutW,
    })

    var received [][]byte
    iface.OnSend(func(pkt []byte) error {
        received = append(received, pkt)
        return nil
    })

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go iface.Start(ctx)

    // Write a test frame
    var enc rnspipe.Encoder
    stdinW.Write(enc.Encode([]byte("hello")))

    // ... assert received[0] == []byte("hello")
}
```
