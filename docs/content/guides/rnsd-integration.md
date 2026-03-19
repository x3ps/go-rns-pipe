---
title: rnsd Integration
weight: 1
description: "Integrate go-rns-pipe with the Reticulum daemon (rnsd)."
---

This guide explains how `go-rns-pipe` programs integrate with the Reticulum daemon (`rnsd`).

## How rnsd Spawns a PipeInterface

`rnsd` uses `PipeInterface` to delegate transport handling to an external process. The flow is:

```
rnsd  ──stdin/stdout──  your-binary
         HDLC frames
```

1. rnsd forks your binary with `stdin` and `stdout` connected to a pipe.
2. rnsd writes HDLC-framed RNS packets to the binary's `stdin`.
3. The binary reads, decodes, and forwards packets to the actual transport (TCP, UDP, serial, etc.).
4. When the transport delivers a packet, the binary HDLC-encodes it and writes to `stdout`.
5. rnsd reads from the binary's `stdout` and processes the decoded packet.

## Reticulum Config

Add a `PipeInterface` section to `~/.reticulum/config`:

```ini
[[MyInterface]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/my-transport --name MyInterface
```

Key fields:
- `command` — path to your binary (and any arguments)
- `respawn_delay` — seconds rnsd waits before respawning after the process exits

## ExitOnEOF

When rnsd shuts down or closes the pipe, stdin receives EOF. The correct response depends on your use case:

| Scenario | Setting |
|----------|---------|
| Spawned by rnsd (child process) | `ExitOnEOF: true` |
| Long-running daemon (self-managed) | `ExitOnEOF: false` (default) |

With `ExitOnEOF: true`, `Start` returns `ErrPipeClosed` on clean EOF. Your binary should call `os.Exit(0)` so rnsd can respawn it cleanly:

```go
if err := iface.Start(ctx); err != nil {
    if errors.Is(err, rnspipe.ErrPipeClosed) {
        os.Exit(0) // rnsd will respawn after respawn_delay
    }
    log.Fatal(err)
}
```

## Startup Ordering

**Critical:** Register `OnSend` before calling `Start`. If `Start` begins reading stdin before `OnSend` is set, decoded packets are silently dropped (the `cb == nil` check in `readLoop`).

```go
iface.OnSend(handler)   // register first
iface.OnStatus(handler) // optional

iface.Start(ctx)        // start reading stdin
```

For transports that need asynchronous initialization (like TCP connections), use a ready channel:

```go
ready := make(chan struct{})

go func() {
    // ... setup transport ...
    iface.OnSend(handler)
    close(ready) // signal that OnSend is registered
    runTransport(ctx)
}()

<-ready         // wait for OnSend to be registered
iface.Start(ctx)
```

## Signal Handling

Always use `signal.NotifyContext` for clean shutdown:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

iface.Start(ctx) // returns when SIGINT/SIGTERM received
```
