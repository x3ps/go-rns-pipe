# go-rns-pipe

Pipeline primitives for RNS data processing.

## Requirements

- [Go](https://go.dev/) 1.24+
- [Docker](https://www.docker.com/) or [Podman](https://podman.io/) (for E2E tests)
- [golangci-lint](https://golangci-lint.run/) (for linting)

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

## Contributing

1. Make changes
2. Run `make test`
3. Run `make lint`
4. Run `make e2e` when a container runtime is available (or `make e2e-tcp` / `make e2e-udp` individually)
5. Open a PR
