---
title: UDP Transport
weight: 3
description: "Run rns-udp-iface, equivalent to the Python RNS UDPInterface."
---

The `examples/udp/` directory contains `rns-udp-iface`, equivalent to Python RNS's `UDPInterface`.

## Architecture

```
rnsd  ←[HDLC/pipe]→  rns-udp-iface  ←[raw datagram]→  remote peers
```

Unlike TCP, UDP **does not use HDLC framing** on the network side — each datagram boundary naturally delimits a packet.

## Building

```bash
make build-udp
# outputs: bin/rns-udp-iface
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen-addr` | `0.0.0.0:4242` | UDP address to listen on for incoming datagrams |
| `--peer-addr` | `255.255.255.255:4242` | UDP address to send packets to (broadcast or unicast) |
| `--name` | `UDPInterface` | Interface name reported to RNS |
| `--mtu` | `500` | RNS packet MTU in bytes |
| `--log-level` | `info` | Log verbosity: `debug`/`info`/`warn`/`error` |

## Environment Variables

| Variable | Flag equivalent |
|----------|----------------|
| `RNS_UDP_LISTEN_ADDR` | `--listen-addr` |
| `RNS_UDP_PEER_ADDR` | `--peer-addr` |
| `RNS_UDP_NAME` | `--name` |
| `RNS_UDP_MTU` | `--mtu` |
| `RNS_UDP_LOG_LEVEL` | `--log-level` |

CLI flags take priority over environment variables.

## Usage

```bash
rns-udp-iface --listen-addr 0.0.0.0:4243 --peer-addr 192.168.1.255:4243 --name UDPBridge
```

`SO_BROADCAST` is always enabled, so `--peer-addr` can be a broadcast address.

## rnsd Configuration

```ini
[[UDPBridge]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/rns-udp-iface --listen-addr 0.0.0.0:4243 --peer-addr 192.168.1.255:4243 --name UDPBridge
```

## Implementation Highlights

### Stateless Design

UDP is the simplest official example: stateless, no reconnect logic, no client/server split. The entire transport logic lives in `transport.go`.

### Socket Loop

The transport reopens the UDP socket on error (matching the pattern in the TCP example):

```go
for {
    // Resolve peer lazily — tolerates DNS not ready at startup
    peer, err := net.ResolveUDPAddr("udp", cfg.PeerAddr)

    conn, err := openUDPConn(listenAddr) // enables SO_BROADCAST

    // readLoop returns on ctx cancel or socket error
    t.readLoop(loopCtx, conn, iface)

    conn.Close()
    if loopCtx.Err() != nil {
        break
    }
    // reopen on error
}
```

### OnSend Callback

Pipe→UDP forwarding happens in the `OnSend` callback, registered before `iface.Start`:

```go
iface.OnSend(func(pkt []byte) error {
    if len(pkt) > cfg.MTU {
        dropped.Add(1)
        return nil
    }
    _, err := conn.WriteTo(pkt, peerAddr)
    return err
})
```

### Read Loop

UDP→pipe forwarding uses a short read deadline to remain responsive to context cancellation:

```go
conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
n, _, err := conn.ReadFromUDP(buf)
// on timeout: continue; on error: return
iface.Receive(buf[:n])
```

### Drop Counter

Oversized or mid-reconnect drops are counted and logged every 30 seconds.

## Protocol Compatibility

Matches Python `UDPInterface.py`:
- Raw datagrams (no HDLC on the network side)
- `SO_BROADCAST` always enabled
- No source-IP filtering: accepts from all senders
