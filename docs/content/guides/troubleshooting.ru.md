---
title: Устранение неполадок
weight: 5
---

## Потеря пакетов (`DroppedPackets() > 0`)

**Причина:** `OnSend` работает медленно и буфер приёма заполняется, или `Receive` вызывается когда интерфейс офлайн.

**Решение:**
- Увеличьте `ReceiveBufferSize` в `Config` для поглощения всплесков.
- Сделайте `OnSend` неблокирующим (например, передайте обработку goroutine с собственной очередью).
- Проверяйте `IsOnline()` перед вызовом `Receive`, или принимайте и обрабатывайте `ErrOffline`.

```go
cfg := rnspipe.Config{
    ReceiveBufferSize: 64, // default is 16
}
```

## `ErrOffline` из `Receive`

**Причина:** `Receive` был вызван, когда интерфейс выполняет переподключение. Это ожидаемое поведение — пайп к rnsd временно недоступен.

**Решение:** Молча отбросьте пакет или поставьте в очередь с обратным давлением:

```go
if err := iface.Receive(pkt); errors.Is(err, rnspipe.ErrOffline) {
    return // drop — rnsd will retransmit at the routing layer
}
```

## Утечка goroutine / `Start` блокируется после отмены контекста

**Причина:** `Stdin` не реализует `io.Closer`. При отмене контекста библиотека закрывает stdin для разблокировки цикла чтения — но если базовый тип не поддерживает `Close`, чтение блокируется навсегда.

**Решение:** Всегда используйте тип, реализующий `io.Closer` для `Stdin`:
- `io.Pipe` (рекомендуется для тестов)
- `*os.File`
- `net.Conn`

Не передавайте голый `bytes.Reader` или `strings.Reader` в качестве `Stdin`.

## Цикл переподключения не останавливается

**Причина:** `ExitOnEOF` равен `false` (по умолчанию), что правильно для долгоживущих интерфейсов rnsd, которые перезапускают процесс. Если вы запускаете бинарный файл интерфейса самостоятельно и хотите, чтобы он завершался при закрытии пайпа rnsd, установите `ExitOnEOF: true`.

```go
cfg := rnspipe.Config{
    ExitOnEOF: true, // exit instead of reconnecting on EOF
}
```

## `ErrNoHandler` из `Start`

**Причина:** `Start` был вызван до регистрации `OnSend`.

**Решение:** Зарегистрируйте `OnSend` до вызова `Start`:

```go
iface.OnSend(func(pkt []byte) error {
    return transport.Send(pkt)
})

go iface.Start(ctx) // Start after OnSend
```

Бинарные файлы примеров TCP и UDP используют канал `ready` для гарантии этого порядка между goroutine.

## Нет вывода в лог

**Причина:** По умолчанию `LogLevel` равен `slog.LevelInfo`. События уровня debug (декодирование фреймов, попытки переподключения) подавляются.

**Решение:** Установите `LogLevel` в `slog.LevelDebug`:

```go
cfg := rnspipe.Config{
    LogLevel: slog.LevelDebug,
}
```

Или передайте полностью пользовательский `Logger` для маршрутизации логов в предпочтительное хранилище.

## `ErrMaxReconnectAttemptsReached`

**Причина:** `MaxReconnectAttempts` был установлен в ненулевое значение и все попытки исчерпаны.

**Решение:** Для продакшн-использования оставьте `MaxReconnectAttempts` равным `0` (по умолчанию), что обеспечивает бесконечные повторы. Устанавливайте ограничение только когда явно хотите, чтобы процесс завершился после ограниченного числа сбоев.

```go
cfg := rnspipe.Config{
    MaxReconnectAttempts: 0, // 0 = unlimited (default)
}
```
