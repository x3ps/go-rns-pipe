---
title: Custom Transport
weight: 4
description: "Build a custom transport using any io.Reader/io.Writer pair, with a WebSocket example."
---

`go-rns-pipe` works with any `io.Reader`/`io.Writer` pair. This guide shows how to build a custom transport — using WebSocket as an example.

## Pattern

Every transport follows the same three-step pattern:

1. Create `rnspipe.Interface` with custom `Stdin`/`Stdout`
2. Register `OnSend` (transport → rnsd direction)
3. Call `iface.Start(ctx)` and forward received data via `iface.Receive` (rnsd → transport direction)

## WebSocket Example

```go
package main

import (
    "context"
    "io"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "golang.org/x/net/websocket"
    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Use os.Stdin/Stdout for the pipe to rnsd.
    iface := rnspipe.New(rnspipe.Config{
        Name:      "WSBridge",
        ExitOnEOF: true,
    })

    iface.OnStatus(func(online bool) {
        log.Printf("pipe online=%v", online)
    })

    // Dial WebSocket peer.
    ws, err := websocket.Dial("ws://remote.host:8080/rns", "", "http://localhost/")
    if err != nil {
        log.Fatal(err)
    }
    defer ws.Close()

    // Register OnSend: pipe→WS forwarding.
    // Called for each HDLC-decoded packet from rnsd.
    iface.OnSend(func(pkt []byte) error {
        _, err := ws.Write(pkt)
        return err
    })

    // WS→pipe forwarding in a goroutine.
    go func() {
        buf := make([]byte, 1064)
        for {
            n, err := ws.Read(buf)
            if err != nil {
                if err != io.EOF {
                    log.Printf("ws read: %v", err)
                }
                stop() // cancel context to shut down iface.Start
                return
            }
            if err := iface.Receive(buf[:n]); err != nil {
                log.Printf("iface.Receive: %v", err)
            }
        }
    }()

    if err := iface.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Using io.Pipe for Testing

`io.Pipe` lets you inject test data without touching `os.Stdin`/`os.Stdout`:

```go
stdinR, stdinW := io.Pipe()
stdoutR, stdoutW := io.Pipe()

iface := rnspipe.New(rnspipe.Config{
    Stdin:  stdinR,
    Stdout: stdoutW,
})

// Write HDLC-framed test data to stdinW.
var enc rnspipe.Encoder
stdinW.Write(enc.Encode([]byte("hello")))

// Read encoded output from stdoutR.
buf := make([]byte, 100)
n, _ := stdoutR.Read(buf)
```

## Serial Transport Sketch

For RS-232/USB serial devices:

```go
import "go.bug.st/serial"

mode := &serial.Mode{BaudRate: 115200}
port, _ := serial.Open("/dev/ttyUSB0", mode)

iface := rnspipe.New(rnspipe.Config{
    Stdin:     port, // implements io.Reader and io.Closer
    Stdout:    port, // implements io.Writer
    HWMTU:     1064,
    ExitOnEOF: true,
})
```

`Stdin` implementing `io.Closer` is important: when the context is cancelled, the library closes `Stdin` to unblock the internal `io.Copy` goroutine.

## Key Rules

1. **Register `OnSend` before `Start`** — packets arriving before `OnSend` is set are silently dropped.
2. **`Stdin` should implement `io.Closer`** — otherwise the goroutine inside `Start` may leak on context cancellation.
3. **`Receive` is goroutine-safe** — call it from multiple goroutines concurrently without locking.
4. **Check `ErrOffline`** — `Receive` returns `ErrOffline` during reconnect windows; drop and retry later.
