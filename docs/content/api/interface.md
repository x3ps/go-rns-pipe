---
title: Interface
weight: 1
description: "Interface type: reads and writes HDLC packets, reproducing Python PipeInterface behavior."
---

`Interface` is the main type. It reads HDLC-framed packets from `stdin` and writes HDLC-framed packets to `stdout`, matching the behavior of Python `PipeInterface.py`.

## Constructor

### `New`

```go
func New(config Config) *Interface
```

Creates a new `Interface` applying defaults for any zero-value fields (see [Config]({{< ref "/api/config" >}})).

```go
iface := rnspipe.New(rnspipe.Config{
    Name:      "MyInterface",
    ExitOnEOF: true,
})
```

## Lifecycle Methods

### `Start`

```go
func (iface *Interface) Start(ctx context.Context) error
```

Begins reading HDLC-framed packets from `config.Stdin`. Blocks until `ctx` is cancelled or an unrecoverable error occurs.

**Preconditions:**
- `OnSend` must be registered before calling `Start` — returns `ErrNoHandler` otherwise.
- Must not be called on an already-running interface — returns `ErrAlreadyStarted`.

**Behavior:**
- Goes online immediately (no handshake), matching `PipeInterface.py`.
- On read error or clean EOF, attempts reconnection with configured backoff.
- Returns `nil` when `ctx` is cancelled.
- Returns `ErrPipeClosed` when EOF occurs and `ExitOnEOF=true`.
- Returns `ErrMaxReconnectAttemptsReached` when all retries are exhausted.

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer cancel()

if err := iface.Start(ctx); err != nil {
    log.Fatal(err)
}
```

## Callback Registration

### `OnSend`

```go
func (iface *Interface) OnSend(fn func([]byte) error)
```

Registers a callback invoked for each decoded packet read from `stdin`. **Must be set before `Start`.**

The callback receives the raw payload (after HDLC decoding). If the callback returns an error, it is logged as a warning but does not stop the interface.

```go
iface.OnSend(func(pkt []byte) error {
    return myTransport.Send(pkt)
})
```

### `OnStatus`

```go
func (iface *Interface) OnStatus(fn func(bool))
```

Registers a callback invoked on every online/offline transition. The `bool` argument is `true` when going online, `false` when going offline.

```go
iface.OnStatus(func(online bool) {
    log.Printf("interface online=%v", online)
})
```

## Sending Packets

### `Receive`

```go
func (iface *Interface) Receive(packet []byte) error
```

HDLC-encodes `packet` and writes it to `config.Stdout` (towards rnsd). Despite the name (which matches the Python PipeInterface API), this is **outbound** from the caller's perspective.

**Errors:**
- `ErrNotStarted` — interface has not been started
- `ErrOffline` — interface is started but currently offline (during a reconnect window)
- `io.ErrShortWrite` — partial write to stdout

Safe to call from multiple goroutines concurrently.

```go
if err := iface.Receive(pkt); err != nil {
    log.Printf("send error: %v", err)
}
```

## Status / Metrics

### `IsOnline`

```go
func (iface *Interface) IsOnline() bool
```

Returns `true` if the interface is currently online.

### `Name`

```go
func (iface *Interface) Name() string
```

Returns the configured interface name.

### `MTU`

```go
func (iface *Interface) MTU() int
```

Returns the configured MTU (default: `500`).

### `HWMTU`

```go
func (iface *Interface) HWMTU() int
```

Returns the configured hardware MTU (default: `1064`).

### Traffic Counters

All counters are atomic (lock-free) and safe to read from any goroutine:

```go
func (iface *Interface) PacketsSent() uint64
func (iface *Interface) PacketsReceived() uint64
func (iface *Interface) BytesSent() uint64
func (iface *Interface) BytesReceived() uint64
```

`BytesSent` reflects payload bytes before HDLC framing. `BytesReceived` reflects payload bytes after HDLC decoding.
