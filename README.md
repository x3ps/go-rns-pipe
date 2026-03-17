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
├── pipe.go              # Library source
├── pipe_test.go         # Tests
├── example/main.go      # Example binary (nix build target)
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
| `update`| Update gomod2nix.toml after changing deps      |
| `lint`  | Run linters                                    |
| `test`  | Run tests                                      |
| `check` | Run nix flake check                            |

## Contributing

1. Enter the dev shell (`nix develop` or `direnv allow`)
2. Make changes
3. Run `task test` and `task lint`
4. Open a PR
