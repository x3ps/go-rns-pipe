---
title: Быстрый старт
weight: 2
---

В этом руководстве показана минимальная настройка для проброса интерфейса Reticulum через `stdin`/`stdout`.

## Минимальный пример

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    iface := rnspipe.New(rnspipe.Config{
        Name:      "MyInterface",
        MTU:       500,
        ExitOnEOF: true, // exit when rnsd closes the pipe; rnsd will respawn us
    })

    // OnSend is called for every HDLC-framed packet decoded from stdin.
    // This is traffic arriving FROM rnsd TO your transport.
    iface.OnSend(func(pkt []byte) error {
        // Forward pkt to your transport (TCP, UDP, serial, etc.)
        log.Printf("→ transport: %d bytes", len(pkt))
        return nil
    })

    // OnStatus is called on every online/offline transition.
    iface.OnStatus(func(online bool) {
        log.Printf("interface online=%v", online)
    })

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Start blocks until ctx is cancelled or an unrecoverable error occurs.
    if err := iface.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Отправка пакетов в rnsd

Вызовите `iface.Receive`, чтобы передать пакет в RNS pipe (имя соответствует Python PipeInterface API):

```go
// data arrives from your transport layer
data := []byte{...}

if err := iface.Receive(data); err != nil {
    log.Printf("send error: %v", err)
}
```

`Receive` кодирует пакет в HDLC и записывает его в `stdout`, откуда читает rnsd.

## Конфигурация rnsd

Добавьте секцию `PipeInterface` в файл `~/.reticulum/config`:

```ini
[[MyInterface]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /path/to/my-transport
```

rnsd запустит ваш бинарный файл, подключив его `stdin`/`stdout` к пайпу. Когда процесс завершится (например, при `ErrPipeClosed`), rnsd перезапустит его после `respawn_delay` секунд.

## Следующие шаги

- [Руководство по TCP-транспорту]({{< ref "/guides/tcp-transport" >}}) — готовый пример TCP-моста
- [Руководство по UDP-транспорту]({{< ref "/guides/udp-transport" >}}) — пример UDP-моста
- [Справочник Config]({{< ref "/api/config" >}}) — все параметры конфигурации
