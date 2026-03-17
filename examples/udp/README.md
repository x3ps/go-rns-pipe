# rns-udp-iface

A minimal UDP transport for [Reticulum](https://reticulum.network/), built on
[go-rns-pipe](../../README.md). This is the simplest official example — stateless,
no reconnect logic, no client/server split.

## Data flow

```
rnsd ←[HDLC/pipe stdin/stdout]→ rns-udp-iface ←[raw UDP datagrams]→ remote peers
```

Key protocol difference from TCP: **no HDLC framing on the UDP side**. Each UDP
datagram is a raw RNS packet — datagram boundaries delimit packets naturally.
`SO_BROADCAST` is always enabled, so both unicast and broadcast peers work.

## Quick start

```sh
# Build
go build -o rns-udp-iface .

# Run (rnsd spawns this as a subprocess via PipeInterface)
./rns-udp-iface --listen-addr 0.0.0.0:4242 --peer-addr 255.255.255.255:4242

# Or with Nix
nix run github:x3ps/go-rns-pipe#rns-udp-iface -- --help
```

## Configuration

All options can be set via flags, environment variables, or both (flags win).

| Flag            | Environment variable    | Default                   | Description                          |
|-----------------|-------------------------|---------------------------|--------------------------------------|
| `--listen-addr` | `RNS_UDP_LISTEN_ADDR`   | `0.0.0.0:4242`            | UDP address to listen on             |
| `--peer-addr`   | `RNS_UDP_PEER_ADDR`     | `255.255.255.255:4242`    | UDP address to send packets to       |
| `--name`        | `RNS_UDP_NAME`          | `UDPInterface`            | Interface name reported to RNS       |
| `--mtu`         | `RNS_UDP_MTU`           | `500`                     | RNS packet MTU in bytes              |
| `--log-level`   | `RNS_UDP_LOG_LEVEL`     | `info`                    | Log level: debug, info, warn, error  |

## Reticulum config snippet

```toml
[[interfaces]]
  [[interfaces.UDPInterface]]
  type = PipeInterface
  enabled = yes
  name = UDPInterface
  command = rns-udp-iface --listen-addr 0.0.0.0:4242 --peer-addr 255.255.255.255:4242
```

## See also

- [examples/tcp](../tcp/README.md) — TCP transport with reconnect and client/server modes
- [go-rns-pipe](../../README.md) — root library documentation
