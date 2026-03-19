---
title: Development Setup
weight: 1
description: "Set up a local development environment for go-rns-pipe."
---

## Requirements

- Go 1.26+
- `golangci-lint` (for linting)
- Python 3.10+ with `rns` package (for E2E tests only)

## Go Path

```bash
git clone https://github.com/x3ps/go-rns-pipe
cd go-rns-pipe
go test ./...
```

No external Go dependencies — everything builds with the standard library.

## Nix Path (Recommended)

The repository ships a `flake.nix` providing a fully reproducible development environment:

```bash
nix develop
```

This shell includes:
- Go 1.26
- `golangci-lint`
- Python (for E2E tests)
- All tools for `make` targets

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make test` | Run all unit tests |
| `make test-root` | Run tests requiring root (raw sockets) |
| `make lint` | Run `golangci-lint` |
| `make build` | Build all examples |
| `make build-tcp` | Build `rns-tcp-iface` only |
| `make build-udp` | Build `rns-udp-iface` only |
| `make e2e` | Run all end-to-end tests |
| `make e2e-tcp` | Run TCP E2E tests |
| `make e2e-udp` | Run UDP E2E tests |
| `make test-examples` | Run example tests |
