# go-rns-pipe

[![CI](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml/badge.svg)](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/x3ps/go-rns-pipe.svg)](https://pkg.go.dev/github.com/x3ps/go-rns-pipe)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

Go implementation of the [Reticulum](https://reticulum.network/) PipeInterface protocol.
Provides HDLC-framed I/O over any `io.Reader`/`io.Writer` pair, wire-compatible
with Python [`PipeInterface.py`](https://github.com/markqvist/Reticulum/blob/master/RNS/Interfaces/PipeInterface.py).

## Installation

```bash
go get github.com/x3ps/go-rns-pipe
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "os/signal"
    "syscall"

    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    iface := rnspipe.New(rnspipe.Config{
        Name:      "MyPipe",
        ExitOnEOF: true, // exit when rnsd closes the pipe
    })

    // Packets decoded from stdin (from rnsd) arrive here.
    iface.OnSend(func(pkt []byte) error {
        log.Printf("received %d bytes from rnsd", len(pkt))
        return nil
    })

    iface.OnStatus(func(online bool) {
        log.Printf("interface online: %v", online)
    })

    // Inject a packet toward rnsd (HDLC-encoded to stdout).
    // iface.Receive(packet)

    if err := iface.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## API Overview

### Interface Methods

| Method | Signature | Description |
|---|---|---|
| `New` | `New(Config) *Interface` | Create an interface with defaults applied |
| `Start` | `Start(ctx) error` | Block reading HDLC frames from Stdin; reconnects on failure. Requires `OnSend` to be registered first |
| `Receive` | `Receive([]byte) error` | HDLC-encode a packet and write it to Stdout (toward rnsd) |
| `SetOnline` | `SetOnline(bool)` | Signal transport-layer up/down (see below) |
| `OnSend` | `OnSend(func([]byte) error)` | Register callback for packets decoded from Stdin |
| `OnStatus` | `OnStatus(func(bool))` | Register callback for online/offline transitions |
| `IsOnline` | `IsOnline() bool` | Whether the interface is currently online |
| `Name` | `Name() string` | Interface name |
| `MTU` | `MTU() int` | Configured MTU |
| `HWMTU` | `HWMTU() int` | Configured hardware MTU |

### Online state

The interface tracks two independent bits:

- **pipe side** — managed internally by `Start`/`readLoop`; goes down on read errors and up on reconnect.
- **transport side** — managed by the caller via `SetOnline`; defaults to `true`.

The effective online state (returned by `IsOnline`, checked by `Receive`, reported by `OnStatus`) is `pipe && transport`. This means a transport adapter can independently signal network up/down without interfering with the pipe reconnect logic.

```go
// TCP disconnect — stop accepting packets from rnsd.
iface.SetOnline(false)

// TCP reconnected — resume normal operation.
iface.SetOnline(true)
```

`OnStatus` fires only when the effective state actually changes, so redundant `SetOnline` calls do not produce spurious callbacks. Callers that never invoke `SetOnline` observe no change in behaviour (transport side stays `true`).

### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `Name` | `string` | `"PipeInterface"` | Interface name for RNS logs |
| `MTU` | `int` | `500` | RNS on-wire MTU in bytes |
| `HWMTU` | `int` | `1064` | Hardware MTU for HDLC buffer sizing |
| `ReconnectDelay` | `time.Duration` | `5s` | Base delay before reconnect attempts |
| `MaxReconnectAttempts` | `int` | `0` (infinite) | Max reconnection attempts; 0 = unlimited |
| `ExponentialBackoff` | `bool` | `false` | Use exponential backoff with jitter (capped at 60s) |
| `ExitOnEOF` | `bool` | `false` | Return `ErrPipeClosed` on clean EOF instead of reconnecting |
| `ReceiveBufferSize` | `int` | `64` | Internal decoded-packet channel capacity |
| `LogLevel` | `slog.Level` | `INFO` | Log verbosity |
| `Logger` | `*slog.Logger` | `nil` (auto) | Custom structured logger |
| `Stdin` | `io.Reader` | `os.Stdin` | Source of HDLC-framed packets from rnsd |
| `Stdout` | `io.Writer` | `os.Stdout` | Destination for HDLC-framed packets to rnsd |

### HDLC Encoder/Decoder

The `Encoder` and `Decoder` are available for building custom transports:

- **`Encoder.Encode([]byte) []byte`** — wrap a packet in HDLC framing (FLAG + escaped data + FLAG)
- **`NewDecoder(hwMTU, chanSize) *Decoder`** — create a streaming decoder
- **`Decoder.Write([]byte) (int, error)`** — feed raw bytes (implements `io.Writer` for use with `io.Copy`)
- **`Decoder.Packets() <-chan []byte`** — channel of decoded packets
- **`Decoder.Close()`** — close the packets channel
- **`Decoder.DroppedPackets() uint64`** — count of packets dropped due to full channel

### Errors

| Error | Description |
|---|---|
| `ErrNotStarted` | Operation attempted before `Start` |
| `ErrAlreadyStarted` | `Start` called on a running interface |
| `ErrNoHandler` | `Start` called without registering `OnSend` first |
| `ErrMaxReconnectAttemptsReached` | All reconnect attempts exhausted |
| `ErrOffline` | `Receive` called while interface is offline (e.g. during reconnect) |
| `ErrPipeClosed` | Clean EOF with `ExitOnEOF=true`; rnsd closed the pipe |

## rnsd Integration

Add a `PipeInterface` section to `~/.reticulum/config`:

```ini
[interfaces]
  [[My Go Pipe]]
    type = PipeInterface
    interface_enabled = Yes
    command = /path/to/your-binary
    respawn_delay = 5
```

The binary communicates with rnsd over stdin/stdout using HDLC framing.

## Development

### Requirements

- [Go](https://go.dev/) 1.26+
- [golangci-lint](https://golangci-lint.run/) (for linting)

Or use the Nix development shell:

```bash
nix develop   # provides go, golangci-lint
```

### Make targets

| Target | Description |
|---|---|
| `make test` | Run unit tests (includes race detector) |
| `make lint` | Run `go vet` and `golangci-lint` |

## Third-party components

The main library has **no external Go dependencies** — it uses only the Go standard library.

### Protocol reference

| Component | Author | License | Repository |
|---|---|---|---|
| Reticulum Network Stack | Mark Qvist | MIT | [markqvist/Reticulum](https://github.com/markqvist/Reticulum) |

This library implements the PipeInterface wire protocol defined by Reticulum. No Reticulum source code is copied or included.

### Development tools

| Tool | Author | License | Repository |
|---|---|---|---|
| Go | The Go Authors | BSD-3-Clause | [golang/go](https://github.com/golang/go) |
| golangci-lint | golangci | MIT | [golangci/golangci-lint](https://github.com/golangci/golangci-lint) |

## License

[AGPL-3.0](LICENSE)
