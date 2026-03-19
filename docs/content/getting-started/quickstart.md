---
title: Quick Start
weight: 2
---

This guide shows the minimal setup to bridge a Reticulum interface over `stdin`/`stdout`.

## Minimal Example

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    iface := rnspipe.New(rnspipe.Config{
        Name:      "MyInterface",
        MTU:       500,
        ExitOnEOF: true, // exit when rnsd closes the pipe; rnsd will respawn us
    })

    // OnSend is called for every HDLC-framed packet decoded from stdin.
    // This is traffic arriving FROM rnsd TO your transport.
    iface.OnSend(func(pkt []byte) error {
        // Forward pkt to your transport (TCP, UDP, serial, etc.)
        log.Printf("→ transport: %d bytes", len(pkt))
        return nil
    })

    // OnStatus is called on every online/offline transition.
    iface.OnStatus(func(online bool) {
        log.Printf("interface online=%v", online)
    })

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Start blocks until ctx is cancelled or an unrecoverable error occurs.
    if err := iface.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Sending Packets to rnsd

Call `iface.Receive` to inject a packet into the RNS pipe (the name matches the Python PipeInterface API):

```go
// data arrives from your transport layer
data := []byte{...}

if err := iface.Receive(data); err != nil {
    log.Printf("send error: %v", err)
}
```

`Receive` HDLC-encodes the packet and writes it to `stdout` where rnsd is reading.

## rnsd Configuration

Add a `PipeInterface` entry to your `~/.reticulum/config`:

```ini
[[MyInterface]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /path/to/my-transport
```

rnsd will spawn your binary, connecting its `stdin`/`stdout` to the pipe. When the process exits (e.g. on `ErrPipeClosed`), rnsd respawns it after `respawn_delay` seconds.

## Next Steps

- [TCP Transport guide]({{< ref "/guides/tcp-transport" >}}) — production TCP bridge example
- [UDP Transport guide]({{< ref "/guides/udp-transport" >}}) — UDP bridge example
- [Config reference]({{< ref "/api/config" >}}) — all configuration options
