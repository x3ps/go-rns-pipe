---
title: Установка
weight: 1
---

## Требования

- Go **1.26** или новее
- Внешние Go-зависимости отсутствуют — только стандартная библиотека

## Установка

```bash
go get github.com/x3ps/go-rns-pipe
```

Импорт в коде:

```go
import rnspipe "github.com/x3ps/go-rns-pipe"
```

## Оболочка разработки Nix

В репозитории есть `flake.nix`, предоставляющий воспроизводимую среду разработки с Go 1.26, `golangci-lint` и Python (для E2E-тестов):

```bash
nix develop
```

Все цели `make` работают внутри Nix-оболочки:

```bash
make test          # юнит-тесты
make lint          # golangci-lint
make build         # сборка примеров
make e2e           # сквозные тесты (требуют Python + rnsd)
```

## Проверка установки

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
