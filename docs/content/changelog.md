---
title: Changelog
weight: 6
description: "Release history and changelog for go-rns-pipe."
---

## Unreleased

### Go 1.26 Upgrade
- Updated module to Go 1.26

### E2E Test Suite Expansion
- Extended resource transfer tests with metadata and varied sizes
- Comprehensive parity and fidelity tests for byte-exact wire compatibility
- Link lifecycle and stress tests (many packets, many concurrent links)
- Channel message ordering and large payload tests
- Shared conftest helpers for Python E2E tests (`establish_link`, `resolve_packet_dest`)

### Test Infrastructure Improvements
- Reconnect and concurrent `Receive` tests (`pipe_test.go`)
- Fuzzing and boundary condition tests for HDLC (`hdlc_test.go`)
- Thread-safety tests for `Decoder`
- `ErrNoHandler` guard: `Start` returns error if `OnSend` not registered before starting
- Integration tests updated for the `ErrNoHandler` requirement

### Bug Fixes
- **goroutine leak:** replaced `time.After` with `time.NewTimer` to prevent timer goroutine leaks
- **HDLC thread-safety:** added mutex protection to `Decoder`

### UDP Transport
- Fixed step ordering in transport startup
- Added proper context lifecycle management
- `pipeConfig` testability seam for injecting `io.Pipe` pairs in tests
- Pipe close test

### TCP Transport
- Register `OnSend` handler before `Start` to satisfy the `ErrNoHandler` requirement

### Documentation
- Added third-party components section with licenses (`NOTICE`)
