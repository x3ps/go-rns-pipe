# go-rns-pipe

Pipeline primitives for RNS data processing.

## Requirements

- [Nix](https://nixos.org/) with flakes enabled
- [direnv](https://direnv.net/) (optional, for automatic shell activation)

## Quick Start

```bash
# Enter development shell
nix develop

# Or with direnv
direnv allow

# Run tests
go test ./...

# Build example binary
nix build
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
│   └── tcp/             # rns-tcp-iface example binary (nix build target)
│       ├── client.go
│       ├── config.go
│       ├── main.go
│       ├── server.go
│       └── transport.go
├── flake.nix            # Nix flake (devShell, packages, checks)
├── Taskfile.yml         # Task runner commands
└── .github/workflows/   # CI and release pipelines
```

## Tasks

Run tasks with [go-task](https://taskfile.dev/):

| Task    | Description                                    |
| ------- | ---------------------------------------------- |
| `dev`   | Enter development shell                        |
| `build` | Build the example binary                       |
| `update`| Regenerate gomod2nix.toml (CLI tool only; not used for build) |
| `lint`  | Run linters                                    |
| `test`      | Run tests                                      |
| `test-all`  | Run all tests including examples/tcp           |
| `build-tcp` | Build rns-tcp-iface with go directly           |
| `check`     | Run nix flake check                            |

## Contributing

1. Enter the dev shell (`nix develop` or `direnv allow`)
2. Make changes
3. Run `task test` and `task lint`
4. Open a PR
