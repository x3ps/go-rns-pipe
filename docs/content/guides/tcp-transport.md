---
title: TCP Transport
weight: 2
---

The `examples/tcp/` directory contains a production-ready TCP transport (`rns-tcp-iface`) that bridges HDLC/pipe traffic to TCP peers.

## Architecture

```
rnsd  ←[HDLC/pipe]→  rns-tcp-iface  ←[HDLC/TCP]→  remote peer(s)
```

HDLC framing is used on **both** sides — the pipe to rnsd and the TCP connections. Both sides reuse the library's `Encoder` and `Decoder`.

## Modes

The binary supports two modes selected with `--mode`:

| Mode | Description |
|------|-------------|
| `client` | Connects to a remote TCP server, reconnects with fixed 5s delay on disconnect |
| `server` | Accepts multiple clients; broadcasts pipe→TCP packets to all connected clients |

## Building

```bash
make build-tcp
# outputs: bin/rns-tcp-iface
```

Or with Go directly:

```bash
go build -o rns-tcp-iface ./examples/tcp/
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mode` | (required) | `client` or `server` |
| `--listen-addr` | `:4242` | Listen address (server mode) |
| `--peer-addr` | (required in client) | Remote TCP address to connect to |
| `--name` | `TCPInterface` | Interface name reported to RNS |
| `--mtu` | `500` | RNS packet MTU in bytes |
| `--reconnect-delay` | `5s` | Base reconnect delay (client mode) |
| `--log-level` | `info` | Log verbosity: `debug`/`info`/`warn`/`error` |

## Environment Variables

| Variable | Flag equivalent |
|----------|----------------|
| `RNS_TCP_MODE` | `--mode` |
| `RNS_TCP_NAME` | `--name` |
| `RNS_TCP_LISTEN_ADDR` | `--listen-addr` |
| `RNS_TCP_PEER_ADDR` | `--peer-addr` |

CLI flags take priority over environment variables.

## Usage

**Client mode** (connect to a remote RNS node):

```bash
rns-tcp-iface --mode client --peer-addr remote.host:4242 --name TCPClient
```

**Server mode** (accept connections from remote nodes):

```bash
rns-tcp-iface --mode server --listen-addr 0.0.0.0:4242 --name TCPServer
```

## rnsd Configuration

```ini
[[TCPBridge]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/rns-tcp-iface --mode client --peer-addr remote.host:4242 --name TCPBridge
```

## Implementation Highlights

### Startup Ordering

The binary uses a `ready` channel to guarantee `OnSend` is registered before `Start` reads stdin:

```go
ready := make(chan struct{})

go func() {
    // runClient/runServer registers OnSend then closes ready
    errc <- runClient(ctx, cfg, iface, logger, ready)
}()

<-ready           // OnSend is registered; safe to start
go func() { errc <- iface.Start(ctx) }()
```

### TCP Socket Options

Matching `TCPInterface.py` defaults:

```go
conn.SetNoDelay(true)            // TCP_NODELAY
conn.SetKeepAlive(true)          // SO_KEEPALIVE
conn.SetKeepAlivePeriod(5*time.Second) // TCP_KEEPIDLE=5s
```

On Linux, also sets `TCP_KEEPINTVL=2s`, `TCP_KEEPCNT=12`, `TCP_USER_TIMEOUT=24s`.

### Hardware MTU

The TCP decoder uses `HW_MTU = 262144` (matching `TCPInterface.py`), larger than the pipe-side `HWMTU = 1064`:

```go
const tcpHWMTU = 262144
decoder := rnspipe.NewDecoder(tcpHWMTU, 64)
```

### Write Deadlines

A 5s write deadline prevents slow clients from blocking the broadcast loop:

```go
conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
conn.Write(frame)
```

## Protocol Compatibility

The TCP framing is identical to Python `TCPInterface.py`:
- HDLC framing: `FLAG=0x7E`, `ESC=0x7D`, `ESC_MASK=0x20`
- No handshake on connect — raw HDLC immediately
- `TCP_NODELAY` enabled

This means `rns-tcp-iface` in client mode can connect to a Python `TCPServerInterface`, and vice versa.
