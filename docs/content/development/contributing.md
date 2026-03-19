---
title: Contributing
weight: 3
---

## Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make changes, add tests
4. Run `make test` and `make lint`
5. Open a PR against `main`

## Code Style

- Standard Go formatting: `gofmt` / `goimports`
- `golangci-lint` must pass (`make lint`)
- No external dependencies — standard library only
- New public APIs must have godoc comments
- Match the behavioral comments style: reference Python `PipeInterface.py` line numbers where applicable

## Commit Format

Conventional Commits:

```
type(scope): short description

Optional body.
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `ci`, `chore`

Scopes: `hdlc`, `pipe`, `config`, `reconnect`, `tcp`, `udp`

Examples:
```
feat(hdlc): add per-decoder packet statistics
fix(pipe): prevent goroutine leak on context cancel without io.Closer
test(hdlc): add fuzzing for malformed escape sequences
docs: add WebSocket custom transport example
```

## Compatibility Requirement

All changes to `hdlc.go` must remain wire-compatible with Python `PipeInterface.py`. Run the E2E tests to verify:

```bash
make e2e
```

## Reporting Bugs

Open an issue at https://github.com/x3ps/go-rns-pipe/issues with:
- Go version (`go version`)
- RNS version if relevant (`python3 -c "import RNS; print(RNS.__version__)"`)
- Minimal reproduction case
