---
title: Errors
weight: 4
---

All sentinel errors are defined in `errors.go`. Use `errors.Is` for comparison.

## Error Variables

### `ErrNotStarted`

```go
var ErrNotStarted = errors.New("interface not started")
```

Returned by `Receive` when called before `Start`.

### `ErrAlreadyStarted`

```go
var ErrAlreadyStarted = errors.New("interface already started")
```

Returned by `Start` when the interface is already running.

### `ErrNoHandler`

```go
var ErrNoHandler = errors.New("OnSend handler not registered")
```

Returned by `Start` when `OnSend` has not been registered. The handler must be set before `Start` to avoid silent packet loss.

### `ErrMaxReconnectAttemptsReached`

```go
var ErrMaxReconnectAttemptsReached = errors.New("max reconnect attempts reached")
```

Returned by `Start` when `MaxReconnectAttempts > 0` and all attempts are exhausted.

### `ErrOffline`

```go
var ErrOffline = errors.New("interface offline")
```

Returned by `Receive` when the interface is started but currently offline — e.g., during the reconnect window between subprocess respawns.

### `ErrPipeClosed`

```go
var ErrPipeClosed = errors.New("pipe closed by remote")
```

Returned by `Start` when stdin reaches a clean EOF and `ExitOnEOF=true`. Signals that rnsd closed the pipe intentionally; the process should exit so rnsd can respawn it via `respawn_delay`.

## Error Handling Pattern

```go
err := iface.Start(ctx)
switch {
case err == nil:
    // clean shutdown via context cancellation
case errors.Is(err, rnspipe.ErrPipeClosed):
    // rnsd closed the pipe — exit for respawn
    os.Exit(0)
case errors.Is(err, rnspipe.ErrMaxReconnectAttemptsReached):
    log.Fatal("gave up reconnecting")
default:
    log.Fatalf("unexpected error: %v", err)
}
```

```go
if err := iface.Receive(pkt); err != nil {
    if errors.Is(err, rnspipe.ErrOffline) {
        // drop and wait — interface will come back
        return
    }
    log.Printf("send error: %v", err)
}
```
