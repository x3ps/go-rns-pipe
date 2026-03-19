---
title: Пользовательский транспорт
weight: 4
---

`go-rns-pipe` работает с любой парой `io.Reader`/`io.Writer`. В этом руководстве показано, как создать пользовательский транспорт — на примере WebSocket.

## Шаблон

Каждый транспорт следует одному и тому же трёхшаговому шаблону:

1. Создайте `rnspipe.Interface` с пользовательскими `Stdin`/`Stdout`
2. Зарегистрируйте `OnSend` (направление transport → rnsd)
3. Вызовите `iface.Start(ctx)` и перенаправляйте полученные данные через `iface.Receive` (направление rnsd → transport)

## Пример WebSocket

```go
package main

import (
    "context"
    "io"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "golang.org/x/net/websocket"
    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Use os.Stdin/Stdout for the pipe to rnsd.
    iface := rnspipe.New(rnspipe.Config{
        Name:      "WSBridge",
        ExitOnEOF: true,
    })

    iface.OnStatus(func(online bool) {
        log.Printf("pipe online=%v", online)
    })

    // Dial WebSocket peer.
    ws, err := websocket.Dial("ws://remote.host:8080/rns", "", "http://localhost/")
    if err != nil {
        log.Fatal(err)
    }
    defer ws.Close()

    // Register OnSend: pipe→WS forwarding.
    // Called for each HDLC-decoded packet from rnsd.
    iface.OnSend(func(pkt []byte) error {
        _, err := ws.Write(pkt)
        return err
    })

    // WS→pipe forwarding in a goroutine.
    go func() {
        buf := make([]byte, 1064)
        for {
            n, err := ws.Read(buf)
            if err != nil {
                if err != io.EOF {
                    log.Printf("ws read: %v", err)
                }
                stop() // cancel context to shut down iface.Start
                return
            }
            if err := iface.Receive(buf[:n]); err != nil {
                log.Printf("iface.Receive: %v", err)
            }
        }
    }()

    if err := iface.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

## Использование io.Pipe для тестирования

`io.Pipe` позволяет вводить тестовые данные без обращения к `os.Stdin`/`os.Stdout`:

```go
stdinR, stdinW := io.Pipe()
stdoutR, stdoutW := io.Pipe()

iface := rnspipe.New(rnspipe.Config{
    Stdin:  stdinR,
    Stdout: stdoutW,
})

// Write HDLC-framed test data to stdinW.
var enc rnspipe.Encoder
stdinW.Write(enc.Encode([]byte("hello")))

// Read encoded output from stdoutR.
buf := make([]byte, 100)
n, _ := stdoutR.Read(buf)
```

## Набросок серийного транспорта

Для устройств RS-232/USB serial:

```go
import "go.bug.st/serial"

mode := &serial.Mode{BaudRate: 115200}
port, _ := serial.Open("/dev/ttyUSB0", mode)

iface := rnspipe.New(rnspipe.Config{
    Stdin:     port, // implements io.Reader and io.Closer
    Stdout:    port, // implements io.Writer
    HWMTU:     1064,
    ExitOnEOF: true,
})
```

Важно, чтобы `Stdin` реализовывал `io.Closer`: при отмене контекста библиотека закрывает `Stdin` для разблокировки внутренней goroutine `io.Copy`.

## Ключевые правила

1. **Регистрируйте `OnSend` до `Start`** — пакеты, поступившие до установки `OnSend`, молча отбрасываются.
2. **`Stdin` должен реализовывать `io.Closer`** — иначе goroutine внутри `Start` может утечь при отмене контекста.
3. **`Receive` потокобезопасен** — вызывайте его из нескольких goroutine параллельно без блокировок.
4. **Проверяйте `ErrOffline`** — `Receive` возвращает `ErrOffline` в период переподключения; отбрасывайте и повторяйте позже.
