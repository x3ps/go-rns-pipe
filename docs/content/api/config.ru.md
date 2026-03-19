---
title: Config
weight: 2
---

`Config` содержит все параметры конфигурации для `Interface`. Передаётся в `New`. Поля с нулевыми значениями заменяются значениями по умолчанию из `DefaultConfig()`.

## Определение типа

```go
type Config struct {
    Name                 string
    MTU                  int
    HWMTU                int
    ReconnectDelay       time.Duration
    MaxReconnectAttempts int
    LogLevel             slog.Level
    Logger               *slog.Logger
    Stdin                io.Reader
    Stdout               io.Writer
    ReceiveBufferSize    int
    ExponentialBackoff   bool
    ExitOnEOF            bool
}
```

## Поля

### `Name`

Имя интерфейса, отображаемое в журналах RNS.

**По умолчанию:** `"PipeInterface"`

### `MTU`

Максимальный размер передаваемого блока данных в байтах — ограничение размера RNS-пакета на уровне протокола.

**По умолчанию:** `500` (стандартный физический MTU RNS, соответствует `Interface.py`)

> Если `MTU > HWMTU`, `New` записывает предупреждение: пакеты могут быть усечены.

### `HWMTU`

MTU аппаратного уровня, используемый для определения размера буфера HDLC-декодера.

**По умолчанию:** `1064` (соответствует `PipeInterface.py` строка 72: `self.HWMTU = 1064`)

### `ReconnectDelay`

Базовая задержка перед попыткой переподключения после сбоя пайпа.

**По умолчанию:** `5s` (соответствует значению по умолчанию `respawn_delay` в `PipeInterface.py`)

При `ExponentialBackoff=false` (по умолчанию) эта задержка применяется без изменений при каждой попытке.

### `MaxReconnectAttempts`

Максимальное количество попыток переподключения. `0` означает бесконечные повторы.

**По умолчанию:** `0` (бесконечно)

После исчерпания `Start` возвращает `ErrMaxReconnectAttemptsReached`.

### `LogLevel`

Уровень детализации логирования (`slog.Level`).

**По умолчанию:** `slog.LevelInfo` (нулевое значение)

Значения: `slog.LevelDebug`, `slog.LevelInfo`, `slog.LevelWarn`, `slog.LevelError`

### `Logger`

Пользовательский `*slog.Logger`. Если `nil`, создаётся текстовый логгер, записывающий в `stderr`.

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
cfg := rnspipe.Config{Logger: logger}
```

### `Stdin`

Reader, из которого читаются HDLC-пакеты (пакеты от rnsd).

**По умолчанию:** `os.Stdin`

Должен реализовывать `io.Closer`, чтобы отмена context могла разблокировать внутреннюю goroutine `io.Copy`. `os.Stdin` намеренно исключён из пути закрытия.

### `Stdout`

Writer, в который записываются HDLC-пакеты (пакеты для rnsd).

**По умолчанию:** `os.Stdout`

### `ReceiveBufferSize`

Ёмкость внутреннего канала пакетов между HDLC-декодером и диспетчером `onSend`.

**По умолчанию:** `64`

Увеличьте значение, если ваш обратный вызов `OnSend` работает медленно и вы видите предупреждения о потере пакетов.

### `ExponentialBackoff`

При `false` (по умолчанию) использует фиксированный `ReconnectDelay` при каждой попытке, воспроизводя поведение `respawn_delay` в `PipeInterface.py`.

При `true` использует экспоненциальный откат: `delay = ReconnectDelay * 2^(attempt-1)` ±25% джиттер, не более 60 секунд.

### `ExitOnEOF`

При `true` `Start` возвращает `ErrPipeClosed` при чистом EOF в stdin вместо попытки переподключения.

**Используйте это при работе в качестве дочернего процесса, запущенного rnsd.** Процесс завершается, и rnsd перезапускает его через `respawn_delay`.

При `false` (по умолчанию) чистый EOF запускает цикл переподключения — подходит для долгоживущих демонов, управляющих жизненным циклом пайпа самостоятельно.

## Значения по умолчанию

```go
func DefaultConfig() Config {
    return Config{
        Name:              "PipeInterface",
        MTU:               500,
        HWMTU:             1064,
        ReconnectDelay:    5 * time.Second,
        ReceiveBufferSize: 64,
    }
}
```

## Пример

```go
iface := rnspipe.New(rnspipe.Config{
    Name:               "TCPBridge",
    MTU:                500,
    ReconnectDelay:     5 * time.Second,
    ExponentialBackoff: true,
    MaxReconnectAttempts: 10,
    ExitOnEOF:          true,
    LogLevel:           slog.LevelDebug,
})
```
