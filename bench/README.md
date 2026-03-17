# go-rns-pipe Benchmark Suite

Integration benchmarks that exercise go-rns-pipe as a real Reticulum
PipeInterface. Two Docker nodes run `rnsd`; a Go binary (`pipe-bridge`) is the
PipeInterface adapter under test. `bench.py` orchestrates five test scenarios
and produces plots and a Markdown report.

## Architecture

```
┌──────────────────────────────────────────────┐
│              bench.py (host)                 │
│  RNS client → TCPClientInterface → node-a   │
└───────────────────┬──────────────────────────┘
                    │ TCP :4242
┌───────────────────▼──────────────────────────┐
│  rns-node-a (Docker)                         │
│  rnsd                                        │
│    PipeInterface → /bench/pipe-bridge        │
│      (go-rns-pipe loopback + Prometheus)     │
│    TCPServerInterface :4242                  │
└───────────────────┬──────────────────────────┘
                    │ TCP :4242 (Docker net)
┌───────────────────▼──────────────────────────┐
│  rns-node-b (Docker)                         │
│  rnsd                                        │
│    TCPClientInterface → rns-node-a:4242      │
│    TCPServerInterface :4243                  │
│  reflector.py (echo server)                  │
└──────────────────────────────────────────────┘
```

## Prerequisites

- Docker + Docker Compose v2
- Python 3.10+
- Go 1.24 (for building pipe-bridge locally)

## Quick Start

```bash
# 1. Build pipe-bridge locally (for HDLC shim test)
cd bench/pipe-bridge && go build . && cd ../..

# 2. Start containers
cd bench && docker compose up --build -d

# 3. Wait for rnsd initialisation (~15 s)
sleep 15

# 4. Install Python dependencies
pip install -r bench/requirements.txt

# 5. Run benchmarks
python bench/bench.py --docker --output bench/results
```

## Test Scenarios

| # | Name | Description |
|---|------|-------------|
| 1 | Connectivity | Measures time for the reflector destination to appear in the RNS routing table via announce propagation |
| 2 | Packet sweep | Sends N packets at each configured size; records per-packet delivery and latency |
| 3 | Burst | Sends 50 packets back-to-back; measures effective throughput |
| 4 | Reconnect | Kills pipe-bridge inside node-a; measures time until rnsd respawns it (requires `--docker`) |
| 5 | HDLC integrity | Runs pipe-bridge in `--shim-mode`; injects valid, corrupted, truncated, and empty frames; counts decoded outputs |

## Output Files

After a successful run you will find in `--output` (default `./results`):

```
raw.csv                  # per-packet measurements from Test 2
latency_by_size.png      # box plot: latency distribution per size
throughput.png           # bar chart: KB/s per size
cdf_latency.png          # CDF of latency for all sizes
burst_timeline.png       # scatter: packet send times in burst test
reconnect_timeline.png   # bar: pipe-bridge downtime
hdlc_integrity.png       # bar: frames sent vs decoded
summary.png              # 2×3 grid of all plots
report.md                # Markdown report with numeric results
```

## Flags

```
--node-a HOST:PORT   node-a TCP address (default: localhost:4242)
--node-b HOST:PORT   node-b TCP address (default: localhost:4243)
--packets N          packets per size in sweep (default: 500)
--sizes A,B,C        comma-separated payload sizes in bytes (default: 64,256,465,500)
--output DIR         output directory (default: ./results)
--no-plots           skip matplotlib plot generation
--skip-hdlc          skip HDLC integrity test
--docker             enable reconnect test (requires running Docker containers)
```

## Compliance Notes

- `pipe-bridge` implements the RNS PipeInterface protocol exactly as specified
  by `PipeInterface.py`: HDLC frame delimiters `0x7E`, escape byte `0x7D`,
  XOR mask `0x20`, HWMTU 1064, MTU 500.
- The loopback mode (echoing every received frame back via `iface.Receive`)
  is only used for benchmarking. In production, `OnSend` should forward
  packets to an actual transport.
- No RNS cryptographic checks are bypassed. All packets travel through the
  full Reticulum stack on both nodes.

## Interpreting Results

- **Latency** includes full round-trip through rnsd → pipe-bridge (loopback) → rnsd,
  across the Docker bridge network. Expect 1–20 ms for small packets on localhost.
- **Throughput** is limited by the RNS 500-byte MTU and 1 Mbps bitrate guess.
  Do not compare directly to raw TCP throughput.
- **HDLC integrity**: The Go decoder intentionally passes corrupted (non-FLAG,
  non-ESC) bytes through unchanged, matching Python PipeInterface behaviour.
  A corrupted payload will therefore still produce a decoded packet.

## Known Limitations

- bench.py uses a simplex send model (no reply path from reflector) because
  the RNS packet receipt confirmation (`PacketReceipt.DELIVERED`) confirms
  delivery to the next-hop, not to the final destination.
- The reconnect test uses `pkill -f pipe-bridge` inside the container, which
  also kills any concurrent pipe-bridge processes. Run `docker compose up -d`
  first to ensure a clean state.
- Docker Compose v1 (`docker-compose`) is not supported; use `docker compose`
  (v2) or set `COMPOSE_COMMAND` in your environment.
