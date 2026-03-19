---
title: Python RNS Compatibility
weight: 3
description: "Protocol-level compatibility between go-rns-pipe and Python RNS: correspondences and differences."
---

`go-rns-pipe` is designed to be wire-compatible with Python RNS. This page documents the exact correspondences and known differences.

## Compatibility Matrix

| Feature | Python PipeInterface.py | go-rns-pipe | Notes |
|---------|------------------------|-------------|-------|
| HDLC FLAG | `0x7E` | `0x7E` | Identical |
| HDLC ESC | `0x7D` | `0x7D` | Identical |
| HDLC ESC_MASK | `0x20` | `0x20` | Identical |
| Escape order | ESC first, then FLAG | ESC first, then FLAG | Identical |
| HWMTU | `1064` | `1064` (default) | Configurable |
| MTU | `500` | `500` (default) | Configurable |
| Reconnect delay | `respawn_delay=5` | `ReconnectDelay=5s` | Identical behavior |
| Empty frames | delivered | delivered | Identical |
| No handshake on connect | ✓ | ✓ | Identical |

## Behavior Differences

### Reconnect Strategy

| Aspect | Python | Go |
|--------|--------|-----|
| Default strategy | Fixed `respawn_delay` | Fixed `ReconnectDelay` (same) |
| Exponential backoff | Not available | Optional via `ExponentialBackoff=true` |
| Max attempts | Infinite | Configurable via `MaxReconnectAttempts` |

Python PipeInterface relies on rnsd's `respawn_delay` to restart the child process. `go-rns-pipe` can optionally handle reconnection internally without process restart.

### Malformed Escape Sequences

Both implementations pass through unrecognized escape sequences unchanged:

```python
# PipeInterface.py — any byte after ESC is unescaped via XOR
byte ^= HDLC.ESC_MASK
```

```go
// go-rns-pipe — only valid sequences are remapped; others pass through as-is
switch byte_ {
case HDLCFlag ^ HDLCEscMask: // 0x5E → 0x7E
    byte_ = HDLCFlag
case HDLCEscape ^ HDLCEscMask: // 0x5D → 0x7D
    byte_ = HDLCEscape
// no default: byte_ unchanged
}
```

Note: Python XOR-unmaps all bytes (so `0x7D 0xAB` → `0x8B`); Go only remaps the two valid sequences. For well-formed data this is identical; malformed data may differ.

### Traffic Counters

Python `PipeInterface.py` does not maintain traffic counters. `go-rns-pipe` exposes atomic counters:

```go
iface.PacketsSent()
iface.PacketsReceived()
iface.BytesSent()
iface.BytesReceived()
```

### Logging

Python uses `RNS.log`. `go-rns-pipe` uses `log/slog` (structured logging). Supply a custom `*slog.Logger` via `Config.Logger`.

## TCP Compatibility

The `examples/tcp` transport is wire-compatible with Python `TCPInterface.py`:

| Feature | TCPInterface.py | rns-tcp-iface |
|---------|----------------|---------------|
| Framing | HDLC | HDLC |
| Handshake | None | None |
| TCP_NODELAY | ✓ | ✓ |
| SO_KEEPALIVE | ✓ | ✓ |
| TCP_KEEPIDLE | 5s | 5s |
| HW_MTU | 262144 | 262144 |
| Client reconnect | 5s fixed | 5s fixed |

## UDP Compatibility

The `examples/udp` transport is wire-compatible with Python `UDPInterface.py`:

| Feature | UDPInterface.py | rns-udp-iface |
|---------|----------------|---------------|
| Framing on wire | None (raw datagram) | None (raw datagram) |
| SO_BROADCAST | ✓ | ✓ |
| Source filtering | None | None |
