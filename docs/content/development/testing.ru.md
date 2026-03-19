---
title: Тестирование
weight: 2
---

## Юнит-тесты

```bash
make test
# or
go test ./...
```

### Покрытие по файлам

| Файл | Ключевое покрытие |
|------|-----------------|
| `hdlc_test.go` | 11 известных векторов кодирования/декодирования, байтовое экранирование, круговой тест для всех 256 значений байта, побайтовый режим, усечение при превышении размера, мусорные/некорректные/усечённые фреймы, двойное закрытие, запись после закрытия, параллельная запись/закрытие, граница HWMTU ±1, 200 раундов с случайными нагрузками |
| `config_test.go` | значения по умолчанию, ограничение отрицательных значений, предупреждение MTU > HWMTU, пользовательский логгер |
| `pipe_test.go` | жизненный цикл (старт/стоп/рестарт), защита `ErrNoHandler`, параллельный `Receive`, `ErrOffline`, счётчики трафика (`SentPackets`, `ReceivedPackets`, `DroppedPackets`), предотвращение утечек goroutine, `ExitOnEOF` |
| `reconnect_test.go` | фиксированная задержка, нулевая задержка при первой попытке, экспоненциальный рост, границы джиттера, ограничение 60s |
| `integration_test.go` | логирование ошибок `OnSend`, логирование отброшенных пакетов, слив при завершении, переходы `OnStatus`, переподключение с новым stdin, параллельные входящие+исходящие, полные круговые тесты |
| `parity_test.go` | побайтовый круговой тест через встроенный Python-декодер, обработка байтов FLAG/ESC, эхо нескольких фреймов, пустой фрейм |

### Бенчмарки

```bash
go test -bench=. -benchmem ./...
```

| Бенчмарк | Что измеряется |
|----------|----------------|
| `BenchmarkEncode` | Пропускная способность HDLC-кодирования (аллокации на вызов) |
| `BenchmarkDecode` | Пропускная способность HDLC-декодирования потока |
| `BenchmarkRoundTrip` | Суммарная пропускная способность кодирования + декодирования |

Подробности о запуске и интерпретации результатов см. в [Бенчмарках](../benchmarks).

### Детектор гонок

Цель `make test` включает `-race`. Для ручного запуска:

```bash
go test -race ./...
```

## Интеграционные тесты

Интеграционные тесты находятся в `integration_test.go` и запускаются стандартной командой `go test ./...` — никаких тегов сборки не требуется.

Они используют пары `io.Pipe` вместо `os.Stdin`/`os.Stdout`, соединённые через вспомогательную функцию `newTestPipe`:

```go
// newTestPipe creates an Interface wired to io.Pipe pairs.
// Returns the interface, stdin writer (inject data), stdout reader (read outbound frames).
func newTestPipe(t *testing.T, opts ...func(*Config)) (*Interface, *io.PipeWriter, *io.PipeReader)
```

Типичное утверждение ожидает на канале с таймаутом, а не читает синхронно:

```go
iface, stdinW, _ := newTestPipe(t)
iface.OnSend(func(pkt []byte) error {
    received <- pkt
    return nil
})

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
go iface.Start(ctx)

waitOnline(t, iface)

var enc Encoder
_, _ = stdinW.Write(enc.Encode([]byte("hello")))

select {
case pkt := <-received:
    if !bytes.Equal(pkt, []byte("hello")) {
        t.Errorf("got %q, want %q", pkt, "hello")
    }
case <-time.After(2 * time.Second):
    t.Fatal("timeout waiting for packet")
}
```

Вспомогательная функция `waitOnline` опрашивает `iface.IsOnline()` с дедлайном 2 секунды, гарантируя готовность пайпа перед вводом фреймов.

## Тесты паритета

`parity_test.go` встраивает минимальный Python HDLC-декодер/энкодер и запускает его как подпроцесс через `os/exec`. Каждый тест кодирует нагрузку в Go, передаёт её в Python-скрипт и проверяет побайтовое совпадение декодированных данных.

Тесты автоматически пропускаются, если Python 3 или пакет `rns` недоступны:

```
--- SKIP: TestHDLCParityPython (Python not available)
```

Тесты паритета охватывают:
- Побайтовый круговой тест для произвольных нагрузок
- Обработку байтов FLAG (`0x7E`) и ESC (`0x7D`)
- Последовательности из нескольких фреймов
- Граничный случай пустого фрейма

## E2E-тесты

Сквозные тесты используют реальный экземпляр `rnsd` (Python RNS) и проверяют совместимость на уровне протокола:

```bash
make e2e       # all E2E tests
make e2e-tcp   # TCP transport only
make e2e-udp   # UDP transport only
```

**Требования:** Python 3.10+ с установленным `rns` (`pip install rns`).

Категории E2E-тестов:
- Доставка пакетов: базовая отправка/получение
- Упорядоченность в канале: пакеты доставляются в порядке отправки
- Большие нагрузки: пакеты размером до MTU
- Жизненный цикл соединения: установка, использование, разрыв
- Паритет и точность: побайтовое сравнение нагрузки
- Передача ресурсов: метаданные и различные размеры
- Нагрузочные тесты: много пакетов, много соединений

## Случайный фаззинг

`TestEncodeDecodeRandomFuzzing` в `hdlc_test.go` запускает 200 параллельных раундов со случайными нагрузками случайного размера (0–2048 байт), используя фиксированный seed для воспроизводимости:

```bash
go test -run TestEncodeDecodeRandomFuzzing -v ./...
```

Для расширения покрытия добавляйте нагрузки, проверяющие специфические байтовые паттерны, непосредственно в `hdlc_test.go`.

## Написание новых тестов

Используйте `newTestPipe` для создания интерфейса с тестовыми соединениями, затем ожидайте на каналах с таймаутами:

```go
func TestMyFeature(t *testing.T) {
    t.Parallel()

    received := make(chan []byte, 1)
    iface, stdinW, _ := newTestPipe(t)
    iface.OnSend(func(pkt []byte) error {
        received <- append([]byte(nil), pkt...)
        return nil
    })

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go iface.Start(ctx)

    waitOnline(t, iface)

    var enc Encoder
    _, _ = stdinW.Write(enc.Encode([]byte("hello")))

    select {
    case pkt := <-received:
        if !bytes.Equal(pkt, []byte("hello")) {
            t.Errorf("got %q, want %q", pkt, "hello")
        }
    case <-time.After(2 * time.Second):
        t.Fatal("timeout waiting for packet")
    }
}
```

Ключевые моменты:
- Всегда используйте `t.Parallel()` для независимых тестов
- Ожидайте через `select` + таймаут `time.After` — никогда не предполагайте синхронную доставку
- Копируйте срезы перед сохранением (`append([]byte(nil), pkt...)`), если обратный вызов переиспользует буферы
- Используйте `context.WithCancel` и `defer cancel()` для чистой остановки интерфейса
