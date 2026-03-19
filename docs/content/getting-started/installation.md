---
title: Installation
weight: 1
---

## Requirements

- Go **1.26** or later
- No external Go dependencies — standard library only

## Install

```bash
go get github.com/x3ps/go-rns-pipe
```

Import in your code:

```go
import rnspipe "github.com/x3ps/go-rns-pipe"
```

## Nix Development Shell

The repository ships a `flake.nix` that provides a reproducible development environment with Go 1.26, `golangci-lint`, and Python (for E2E tests):

```bash
nix develop
```

All `make` targets work inside the Nix shell:

```bash
make test          # unit tests
make lint          # golangci-lint
make build         # build examples
make e2e           # end-to-end tests (requires Python + rnsd)
```

## Verifying the Installation

```go
package main

import (
    "fmt"
    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    cfg := rnspipe.DefaultConfig()
    fmt.Printf("MTU=%d HWMTU=%d\n", cfg.MTU, cfg.HWMTU)
    // Output: MTU=500 HWMTU=1064
}
```
