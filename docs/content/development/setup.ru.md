---
title: Настройка среды разработки
weight: 1
---

## Требования

- Go 1.26+
- `golangci-lint` (для линтинга)
- Python 3.10+ с пакетом `rns` (только для E2E-тестов)

## Стандартный путь

```bash
git clone https://github.com/x3ps/go-rns-pipe
cd go-rns-pipe
go test ./...
```

Внешние Go-зависимости отсутствуют — всё собирается со стандартной библиотекой.

## Путь через Nix (рекомендуется)

В репозитории есть `flake.nix`, предоставляющий полностью воспроизводимую среду разработки:

```bash
nix develop
```

Эта оболочка включает:
- Go 1.26
- `golangci-lint`
- Python (для E2E-тестов)
- Все инструменты для целей `make`

## Цели Makefile

| Цель | Описание |
|------|----------|
| `make test` | Запустить все юнит-тесты |
| `make test-root` | Запустить тесты, требующие root (сырые сокеты) |
| `make lint` | Запустить `golangci-lint` |
| `make build` | Собрать все примеры |
| `make build-tcp` | Собрать только `rns-tcp-iface` |
| `make build-udp` | Собрать только `rns-udp-iface` |
| `make e2e` | Запустить все сквозные тесты |
| `make e2e-tcp` | Запустить TCP E2E-тесты |
| `make e2e-udp` | Запустить UDP E2E-тесты |
| `make test-examples` | Запустить тесты примеров |
