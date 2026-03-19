---
title: Участие в разработке
weight: 3
---

## Рабочий процесс

1. Форкните репозиторий
2. Создайте ветку для функции: `git checkout -b feat/my-feature`
3. Внесите изменения, добавьте тесты
4. Запустите `make test` и `make lint`
5. Откройте PR в ветку `main`

## Стиль кода

- Стандартное форматирование Go: `gofmt` / `goimports`
- `golangci-lint` должен проходить (`make lint`)
- Без внешних зависимостей — только стандартная библиотека
- Новые публичные API должны иметь комментарии godoc
- Соответствуйте стилю поведенческих комментариев: ссылайтесь на номера строк Python `PipeInterface.py` там, где применимо

## Формат коммитов

Conventional Commits:

```
type(scope): short description

Optional body.
```

Типы: `feat`, `fix`, `refactor`, `test`, `docs`, `ci`, `chore`

Области: `hdlc`, `pipe`, `config`, `reconnect`, `tcp`, `udp`

Примеры:
```
feat(hdlc): add per-decoder packet statistics
fix(pipe): prevent goroutine leak on context cancel without io.Closer
test(hdlc): add fuzzing for malformed escape sequences
docs: add WebSocket custom transport example
```

## Требование совместимости

Все изменения в `hdlc.go` должны сохранять совместимость с Python `PipeInterface.py` на уровне протокола. Запускайте E2E-тесты для проверки:

```bash
make e2e
```

## Сообщение об ошибках

Откройте issue на https://github.com/x3ps/go-rns-pipe/issues, указав:
- Версию Go (`go version`)
- Версию RNS при необходимости (`python3 -c "import RNS; print(RNS.__version__)"`)
- Минимальный воспроизводящий пример
