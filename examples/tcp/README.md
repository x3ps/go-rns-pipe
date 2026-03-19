# rns-tcp-iface

A TCP transport for [Reticulum](https://reticulum.network), equivalent to Python RNS's `TCPClientInterface` / `TCPServerInterface`.

It bridges HDLC-framed traffic between a pipe to `rnsd` (stdin/stdout) and one or more TCP connections:

```
rnsd ←[HDLC/pipe]→ rns-tcp-iface ←[HDLC/TCP]→ remote peer(s)
```

Built on top of [go-rns-pipe](../../README.md).

## Quick Start

**Server** (listens for incoming TCP connections):

```sh
rns-tcp-iface --mode server --listen-addr :4242
```

**Client** (connects to a TCP server):

```sh
rns-tcp-iface --mode client --peer-addr 192.168.1.10:4242
```

## Configuration

| Flag | Environment Variable | Default | Description |
|---|---|---|---|
| `--mode` | `RNS_TCP_MODE` | *(required)* | Operating mode: `client` or `server` |
| `--name` | `RNS_TCP_NAME` | `TCPInterface` | Interface name reported to RNS |
| `--listen-addr` | `RNS_TCP_LISTEN_ADDR` | `:4242` | Listen address (server mode) |
| `--peer-addr` | `RNS_TCP_PEER_ADDR` | | Peer address (client mode, required) |
| `--mtu` | --- | `500` | RNS packet MTU in bytes |
| `--reconnect-delay` | --- | `5s` | Base reconnect delay (client mode) |
| `--log-level` | --- | `info` | Log level: debug, info, warn, error |

CLI flags take precedence over environment variables, which take precedence over defaults.

## rnsd Integration

Add to your `~/.reticulum/config`:

```ini
# Server — accept TCP connections from remote peers
[[TCPServerInterface]]
  type = PipeInterface
  interface_enabled = Yes
  command = rns-tcp-iface --mode server --listen-addr :4242
  respawn_delay = 5

# Client — connect to a remote TCP peer
[[TCPClientInterface]]
  type = PipeInterface
  interface_enabled = Yes
  command = rns-tcp-iface --mode client --peer-addr 10.0.0.1:4242
  respawn_delay = 5
```

## Protocol Details

- HDLC framing on both pipe side (stdin/stdout) and TCP side
- TCP HW_MTU: 262144 bytes (matching `TCPInterface.py`)
- `TCP_NODELAY` and `SO_KEEPALIVE` enabled on all connections
- `TCP_KEEPIDLE` = 5s
- **Linux only:** `TCP_KEEPINTVL` = 2s, `TCP_KEEPCNT` = 12, `TCP_USER_TIMEOUT` = 24s
- Non-Linux platforms use Go defaults for keepalive interval/count (no-op stub)
- Client reconnect: fixed 5s delay between attempts
- Server: broadcasts outbound packets to all connected TCP clients
- Write deadline: 5s per packet (prevents slow clients from blocking)

## Building

```sh
cd examples/tcp && go build -o rns-tcp-iface .
```
