# go-rns-pipe

Pipeline primitives for RNS data processing.

## Requirements

- [Go](https://go.dev/) 1.24+
- [Docker](https://www.docker.com/) or [Podman](https://podman.io/) (for E2E tests)
- [golangci-lint](https://golangci-lint.run/) (for linting)

Or use the Nix development shell (requires [Nix with flakes](https://nixos.wiki/wiki/Flakes)):
```bash
nix develop   # provides go, golangci-lint, docker-compose, python + rns venv
```

## Quick Start

```bash
# Run tests
make test

# Lint
make lint

# Build packaged binaries
make build
```

## Project Structure

```
.
├── config.go            # Config struct and defaults
├── errors.go            # Exported error values
├── hdlc.go              # HDLC encoder/decoder
├── pipe.go              # Interface implementation
├── pipe_test.go         # Tests
├── reconnect.go         # Reconnection with exponential backoff
├── examples/
│   ├── tcp/             # rns-tcp-iface example transport + tests
│   └── udp/             # rns-udp-iface example transport + tests
├── Makefile             # Build, test, and lint targets
└── .github/workflows/   # CI and release pipelines
```

## Architecture Notes

### TCP server broadcast semantics

Upstream `TCPServerInterface.py` spawns a separate `TCPClientInterface` (a distinct rnsd interface
registration) for each accepted TCP client. This process has a single stdin/stdout pipe to rnsd —
one RNS interface for the whole process — so broadcasting to all connected TCP clients is the
correct behaviour. Inbound traffic (client → rnsd via `iface.Receive`) remains per-connection.

### Remaining behavioural differences from official Reticulum

| Area | Difference |
|---|---|
| TCP_KEEPINTVL | Go stdlib sets same value as KEEPIDLE (5s); Python sets interval=2s |
| UDP multicast | Not implemented (only broadcast); matches Python UDPInterface.py |

## Contributing

1. Make changes
2. Run `make test`
3. Run `make lint`
4. Run `make e2e` when a container runtime is available (or `make e2e-tcp` / `make e2e-udp` individually)
5. Open a PR
