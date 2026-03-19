---
title: Config
weight: 2
description: "Config struct: all configuration parameters for the Interface, with defaults."
---

`Config` holds all configuration for an `Interface`. Pass it to `New`. Zero-value fields are replaced with defaults from `DefaultConfig()`.

## Type Definition

```go
type Config struct {
    Name                 string
    MTU                  int
    HWMTU                int
    ReconnectDelay       time.Duration
    MaxReconnectAttempts int
    LogLevel             slog.Level
    Logger               *slog.Logger
    Stdin                io.Reader
    Stdout               io.Writer
    ReceiveBufferSize    int
    ExponentialBackoff   bool
    ExitOnEOF            bool
}
```

## Fields

### `Name`

Interface name as it appears in RNS logs.

**Default:** `"PipeInterface"`

### `MTU`

Maximum transmission unit in bytes — the on-wire RNS packet size limit.

**Default:** `500` (standard RNS physical MTU, matches `Interface.py`)

> If `MTU > HWMTU`, `New` logs a warning: packets may be truncated.

### `HWMTU`

Hardware-level MTU used for HDLC decoder buffer sizing.

**Default:** `1064` (matches `PipeInterface.py` line 72: `self.HWMTU = 1064`)

### `ReconnectDelay`

Base delay before attempting reconnection after a pipe failure.

**Default:** `5s` (matches `PipeInterface.py` `respawn_delay` default)

With `ExponentialBackoff=false` (default), this delay is used unchanged on every attempt.

### `MaxReconnectAttempts`

Maximum number of reconnect attempts. `0` means infinite retries.

**Default:** `0` (infinite)

When exhausted, `Start` returns `ErrMaxReconnectAttemptsReached`.

### `LogLevel`

Verbosity of the default structured logger (`slog.Level`).

**Default:** `slog.LevelInfo` (zero value)

Values: `slog.LevelDebug`, `slog.LevelInfo`, `slog.LevelWarn`, `slog.LevelError`

### `Logger`

Custom `*slog.Logger`. If `nil`, a text-format logger writing to `stderr` is created.

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
cfg := rnspipe.Config{Logger: logger}
```

### `Stdin`

Reader from which HDLC-framed packets are read (packets from rnsd).

**Default:** `os.Stdin`

Should implement `io.Closer` so that context cancellation can unblock the internal `io.Copy` goroutine. `os.Stdin` is deliberately excluded from the close path.

### `Stdout`

Writer to which HDLC-framed packets are written (packets to rnsd).

**Default:** `os.Stdout`

### `ReceiveBufferSize`

Capacity of the internal packet channel between the HDLC decoder and the `onSend` dispatcher.

**Default:** `64`

Increase this if your `OnSend` callback is slow and you see dropped packet warnings.

### `ExponentialBackoff`

When `false` (default), uses a fixed `ReconnectDelay` on every attempt, matching `PipeInterface.py` `respawn_delay` behavior.

When `true`, uses exponential backoff: `delay = ReconnectDelay * 2^(attempt-1)` ±25% jitter, capped at 60 seconds.

### `ExitOnEOF`

When `true`, `Start` returns `ErrPipeClosed` on a clean stdin EOF instead of attempting reconnection.

**Use this when running as a child process spawned by rnsd.** The process exits and rnsd respawns it via `respawn_delay`.

When `false` (default), clean EOF triggers a reconnect loop — suitable for long-running daemons that manage their own pipe lifecycle.

## Defaults

```go
func DefaultConfig() Config {
    return Config{
        Name:              "PipeInterface",
        MTU:               500,
        HWMTU:             1064,
        ReconnectDelay:    5 * time.Second,
        ReceiveBufferSize: 64,
    }
}
```

## Example

```go
iface := rnspipe.New(rnspipe.Config{
    Name:               "TCPBridge",
    MTU:                500,
    ReconnectDelay:     5 * time.Second,
    ExponentialBackoff: true,
    MaxReconnectAttempts: 10,
    ExitOnEOF:          true,
    LogLevel:           slog.LevelDebug,
})
```
