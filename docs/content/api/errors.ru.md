---
title: Ошибки
weight: 4
---

Все сигнальные ошибки определены в `errors.go`. Используйте `errors.Is` для сравнения.

## Переменные ошибок

### `ErrNotStarted`

```go
var ErrNotStarted = errors.New("interface not started")
```

Возвращается `Receive` при вызове до `Start`.

### `ErrAlreadyStarted`

```go
var ErrAlreadyStarted = errors.New("interface already started")
```

Возвращается `Start`, если интерфейс уже запущен.

### `ErrNoHandler`

```go
var ErrNoHandler = errors.New("OnSend handler not registered")
```

Возвращается `Start`, если `OnSend` не был зарегистрирован. Обработчик должен быть установлен до `Start`, чтобы избежать молчаливой потери пакетов.

### `ErrMaxReconnectAttemptsReached`

```go
var ErrMaxReconnectAttemptsReached = errors.New("max reconnect attempts reached")
```

Возвращается `Start`, если `MaxReconnectAttempts > 0` и все попытки исчерпаны.

### `ErrOffline`

```go
var ErrOffline = errors.New("interface offline")
```

Возвращается `Receive`, если интерфейс запущен, но в данный момент офлайн — например, во время окна переподключения между перезапусками подпроцесса.

### `ErrPipeClosed`

```go
var ErrPipeClosed = errors.New("pipe closed by remote")
```

Возвращается `Start`, когда stdin достигает чистого EOF и `ExitOnEOF=true`. Сигнализирует о намеренном закрытии пайпа со стороны rnsd; процесс должен завершиться, чтобы rnsd мог перезапустить его через `respawn_delay`.

## Шаблон обработки ошибок

```go
err := iface.Start(ctx)
switch {
case err == nil:
    // clean shutdown via context cancellation
case errors.Is(err, rnspipe.ErrPipeClosed):
    // rnsd closed the pipe — exit for respawn
    os.Exit(0)
case errors.Is(err, rnspipe.ErrMaxReconnectAttemptsReached):
    log.Fatal("gave up reconnecting")
default:
    log.Fatalf("unexpected error: %v", err)
}
```

```go
if err := iface.Receive(pkt); err != nil {
    if errors.Is(err, rnspipe.ErrOffline) {
        // drop and wait — interface will come back
        return
    }
    log.Printf("send error: %v", err)
}
```
