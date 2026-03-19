---
title: Интеграция с rnsd
weight: 1
---

В этом руководстве описывается, как программы на `go-rns-pipe` интегрируются с демоном Reticulum (`rnsd`).

## Как rnsd запускает PipeInterface

`rnsd` использует `PipeInterface` для делегирования обработки транспорта внешнему процессу. Схема работы:

```
rnsd  ──stdin/stdout──  your-binary
         HDLC frames
```

1. rnsd форкает ваш бинарный файл, подключая `stdin` и `stdout` к пайпу.
2. rnsd записывает HDLC-пакеты RNS в `stdin` бинарного файла.
3. Бинарный файл читает, декодирует и перенаправляет пакеты на реальный транспорт (TCP, UDP, serial и т.д.).
4. Когда транспорт доставляет пакет, бинарный файл HDLC-кодирует его и записывает в `stdout`.
5. rnsd читает из `stdout` бинарного файла и обрабатывает декодированный пакет.

## Конфигурация Reticulum

Добавьте секцию `PipeInterface` в файл `~/.reticulum/config`:

```ini
[[MyInterface]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/my-transport --name MyInterface
```

Ключевые поля:
- `command` — путь к вашему бинарному файлу (и любые аргументы)
- `respawn_delay` — секунды, которые rnsd ждёт перед перезапуском после завершения процесса

## ExitOnEOF

Когда rnsd завершает работу или закрывает пайп, stdin получает EOF. Правильный ответ зависит от вашего варианта использования:

| Сценарий | Настройка |
|----------|---------|
| Запущен rnsd (дочерний процесс) | `ExitOnEOF: true` |
| Долгоживущий демон (самоуправляемый) | `ExitOnEOF: false` (по умолчанию) |

При `ExitOnEOF: true` `Start` возвращает `ErrPipeClosed` при чистом EOF. Ваш бинарный файл должен вызвать `os.Exit(0)`, чтобы rnsd мог корректно его перезапустить:

```go
if err := iface.Start(ctx); err != nil {
    if errors.Is(err, rnspipe.ErrPipeClosed) {
        os.Exit(0) // rnsd will respawn after respawn_delay
    }
    log.Fatal(err)
}
```

## Порядок запуска

**Критично:** Зарегистрируйте `OnSend` до вызова `Start`. Если `Start` начнёт читать stdin до установки `OnSend`, декодированные пакеты будут молча отброшены (проверка `cb == nil` в `readLoop`).

```go
iface.OnSend(handler)   // register first
iface.OnStatus(handler) // optional

iface.Start(ctx)        // start reading stdin
```

Для транспортов, которым требуется асинхронная инициализация (например, TCP-соединения), используйте канал ready:

```go
ready := make(chan struct{})

go func() {
    // ... setup transport ...
    iface.OnSend(handler)
    close(ready) // signal that OnSend is registered
    runTransport(ctx)
}()

<-ready         // wait for OnSend to be registered
iface.Start(ctx)
```

## Обработка сигналов

Всегда используйте `signal.NotifyContext` для чистого завершения:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

iface.Start(ctx) // returns when SIGINT/SIGTERM received
```
