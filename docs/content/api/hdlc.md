---
title: HDLC
weight: 3
---

The `hdlc.go` file provides low-level HDLC framing primitives. Both `Encoder` and `Decoder` are exported so custom transports can reuse the same framing on non-pipe connections (e.g., TCP peers).

## Constants

```go
const (
    HDLCFlag    = 0x7E // Frame delimiter
    HDLCEscape  = 0x7D // Escape character
    HDLCEscMask = 0x20 // XOR mask applied to escaped bytes
)
```

These match `PipeInterface.py` lines 40–42:
```python
class HDLC:
    FLAG    = 0x7E
    ESC     = 0x7D
    ESC_MASK = 0x20
```

## Encoder

### Type

```go
type Encoder struct{}
```

A zero-value `Encoder` is ready to use — no initialization required.

### `Encode`

```go
func (e *Encoder) Encode(packet []byte) []byte
```

Wraps `packet` in HDLC framing: `FLAG + escaped(data) + FLAG`.

**Escape order (critical):** ESC bytes are escaped first, then FLAG bytes. This matches `PipeInterface.py` `HDLC.escape`.

| Input byte | Encoded as        |
|-----------|-------------------|
| `0x7D`    | `0x7D 0x5D`       |
| `0x7E`    | `0x7D 0x5E`       |
| other     | unchanged         |

Returns a new byte slice. Safe to call concurrently on the same `Encoder`.

```go
var enc rnspipe.Encoder
frame := enc.Encode([]byte{0x01, 0x7E, 0x02})
// → [0x7E, 0x01, 0x7D, 0x5E, 0x02, 0x7E]
```

## Decoder

### Type

```go
type Decoder struct { /* unexported fields */ }
```

Stateful stream decoder. Implements `io.Writer` so it can be fed raw bytes from `io.Copy`.

Concurrent `Write` and `Close` calls are safe (protected by an internal mutex).

### `NewDecoder`

```go
func NewDecoder(hwMTU, chanSize int) *Decoder
```

Creates a new `Decoder`.

- `hwMTU` — frames larger than this are silently truncated (matches `PipeInterface.py` `HW_MTU` limit)
- `chanSize` — capacity of the internal packets channel

```go
decoder := rnspipe.NewDecoder(1064, 64)
```

### `Write`

```go
func (d *Decoder) Write(b []byte) (int, error)
```

Feeds raw bytes into the decoder. Decoded complete frames are emitted on the `Packets()` channel.

Returns `io.ErrClosedPipe` after `Close` has been called. Intended to be used with `io.Copy`:

```go
go func() {
    _, err := io.Copy(decoder, conn)
    decoder.Close()
    errCh <- err
}()
```

### `Packets`

```go
func (d *Decoder) Packets() <-chan []byte
```

Returns a read-only channel that emits decoded packet payloads. The channel is closed after `Close` is called and all buffered packets are consumed.

### `Close`

```go
func (d *Decoder) Close()
```

Signals end-of-stream. Closes the packets channel. Safe to call multiple times (idempotent via `sync.Once`).

After `Close`, `Write` returns `io.ErrClosedPipe`.

### `DroppedPackets`

```go
func (d *Decoder) DroppedPackets() uint64
```

Returns the count of packets dropped because the `Packets()` channel was full (non-blocking send failed).

## Usage in Custom Transports

```go
decoder := rnspipe.NewDecoder(262144, 64) // tcpHWMTU, buffer

go func() {
    io.Copy(decoder, tcpConn)
    decoder.Close()
}()

for pkt := range decoder.Packets() {
    // forward pkt to rnsd via iface.Receive(pkt)
}
```
