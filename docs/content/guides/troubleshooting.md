---
title: Troubleshooting
weight: 5
---

## Packets Dropped (`DroppedPackets() > 0`)

**Cause:** `OnSend` is slow and the receive buffer fills up, or `Receive` is called while the interface is offline.

**Fix:**
- Increase `ReceiveBufferSize` in `Config` to absorb bursts.
- Make `OnSend` non-blocking (e.g. hand off to a goroutine with its own queue).
- Check `IsOnline()` before calling `Receive`, or accept and handle `ErrOffline`.

```go
cfg := rnspipe.Config{
    ReceiveBufferSize: 64, // default is 16
}
```

## `ErrOffline` from `Receive`

**Cause:** `Receive` was called while the interface is reconnecting. This is expected behaviour — the pipe to rnsd is temporarily unavailable.

**Fix:** Drop the packet silently, or queue with backpressure:

```go
if err := iface.Receive(pkt); errors.Is(err, rnspipe.ErrOffline) {
    return // drop — rnsd will retransmit at the routing layer
}
```

## Goroutine Leak / `Start` Blocks After Context Cancel

**Cause:** `Stdin` does not implement `io.Closer`. When the context is cancelled, the library closes stdin to unblock the read loop — but if the underlying type does not support `Close`, the read blocks forever.

**Fix:** Always use a type that implements `io.Closer` for `Stdin`:
- `io.Pipe` (recommended for tests)
- `*os.File`
- A `net.Conn`

Do not pass a bare `bytes.Reader` or `strings.Reader` as `Stdin`.

## Reconnect Loop Never Stops

**Cause:** `ExitOnEOF` is `false` (the default), which is correct for long-lived rnsd interfaces that respawn the process. If you spawn the interface binary yourself and want it to exit when rnsd closes the pipe, set `ExitOnEOF: true`.

```go
cfg := rnspipe.Config{
    ExitOnEOF: true, // exit instead of reconnecting on EOF
}
```

## `ErrNoHandler` from `Start`

**Cause:** `Start` was called before `OnSend` was registered.

**Fix:** Register `OnSend` before calling `Start`:

```go
iface.OnSend(func(pkt []byte) error {
    return transport.Send(pkt)
})

go iface.Start(ctx) // Start after OnSend
```

The TCP and UDP example binaries use a `ready` channel to guarantee this ordering across goroutines.

## No Log Output

**Cause:** Default `LogLevel` is `slog.LevelInfo`. Debug-level events (frame decoding, reconnect attempts) are suppressed.

**Fix:** Set `LogLevel` to `slog.LevelDebug`:

```go
cfg := rnspipe.Config{
    LogLevel: slog.LevelDebug,
}
```

Or pass a fully custom `Logger` to route logs to your preferred sink.

## `ErrMaxReconnectAttemptsReached`

**Cause:** `MaxReconnectAttempts` was set to a non-zero value and all attempts were exhausted.

**Fix:** For production use, leave `MaxReconnectAttempts` at `0` (the default), which retries indefinitely. Set a limit only when you explicitly want the process to exit after a bounded number of failures.

```go
cfg := rnspipe.Config{
    MaxReconnectAttempts: 0, // 0 = unlimited (default)
}
```
