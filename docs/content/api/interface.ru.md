---
title: Interface
weight: 1
---

`Interface` — основной тип. Читает HDLC-пакеты из `stdin` и записывает HDLC-пакеты в `stdout`, воспроизводя поведение Python `PipeInterface.py`.

## Конструктор

### `New`

```go
func New(config Config) *Interface
```

Создаёт новый `Interface`, применяя значения по умолчанию для полей с нулевым значением (см. [Config]({{< ref "/api/config" >}})).

```go
iface := rnspipe.New(rnspipe.Config{
    Name:      "MyInterface",
    ExitOnEOF: true,
})
```

## Методы жизненного цикла

### `Start`

```go
func (iface *Interface) Start(ctx context.Context) error
```

Начинает чтение HDLC-пакетов из `config.Stdin`. Блокирует выполнение до отмены `ctx` или возникновения неустранимой ошибки.

**Предусловия:**
- `OnSend` должен быть зарегистрирован перед вызовом `Start` — иначе возвращает `ErrNoHandler`.
- Не должен вызываться на уже запущенном интерфейсе — иначе возвращает `ErrAlreadyStarted`.

**Поведение:**
- Сразу переходит в состояние онлайн (без рукопожатия), как и `PipeInterface.py`.
- При ошибке чтения или чистом EOF выполняет переподключение с заданным откатом.
- Возвращает `nil` при отмене `ctx`.
- Возвращает `ErrPipeClosed` при EOF и `ExitOnEOF=true`.
- Возвращает `ErrMaxReconnectAttemptsReached` при исчерпании всех попыток.

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer cancel()

if err := iface.Start(ctx); err != nil {
    log.Fatal(err)
}
```

## Регистрация обратных вызовов

### `OnSend`

```go
func (iface *Interface) OnSend(fn func([]byte) error)
```

Регистрирует обратный вызов, вызываемый для каждого декодированного пакета, прочитанного из `stdin`. **Должен быть установлен до `Start`.**

Обратный вызов получает чистую нагрузку (после HDLC-декодирования). Если обратный вызов возвращает ошибку, она логируется как предупреждение, но не останавливает интерфейс.

```go
iface.OnSend(func(pkt []byte) error {
    return myTransport.Send(pkt)
})
```

### `OnStatus`

```go
func (iface *Interface) OnStatus(fn func(bool))
```

Регистрирует обратный вызов, вызываемый при каждом переходе онлайн/офлайн. Аргумент `bool` равен `true` при переходе в онлайн, `false` при переходе в офлайн.

```go
iface.OnStatus(func(online bool) {
    log.Printf("interface online=%v", online)
})
```

## Отправка пакетов

### `Receive`

```go
func (iface *Interface) Receive(packet []byte) error
```

Кодирует `packet` в HDLC и записывает его в `config.Stdout` (в направлении rnsd). Несмотря на название (которое соответствует Python PipeInterface API), это **исходящее** направление с точки зрения вызывающей стороны.

**Ошибки:**
- `ErrNotStarted` — интерфейс не был запущен
- `ErrOffline` — интерфейс запущен, но в данный момент офлайн (во время окна переподключения)
- `io.ErrShortWrite` — частичная запись в stdout

Безопасен для параллельных вызовов из нескольких goroutine.

```go
if err := iface.Receive(pkt); err != nil {
    log.Printf("send error: %v", err)
}
```

## Статус / Метрики

### `IsOnline`

```go
func (iface *Interface) IsOnline() bool
```

Возвращает `true`, если интерфейс в данный момент онлайн.

### `Name`

```go
func (iface *Interface) Name() string
```

Возвращает настроенное имя интерфейса.

### `MTU`

```go
func (iface *Interface) MTU() int
```

Возвращает настроенный MTU (по умолчанию: `500`).

### `HWMTU`

```go
func (iface *Interface) HWMTU() int
```

Возвращает настроенный аппаратный MTU (по умолчанию: `1064`).

### Счётчики трафика

Все счётчики атомарны (без блокировок) и безопасны для чтения из любой goroutine:

```go
func (iface *Interface) PacketsSent() uint64
func (iface *Interface) PacketsReceived() uint64
func (iface *Interface) BytesSent() uint64
func (iface *Interface) BytesReceived() uint64
```

`BytesSent` отражает байты нагрузки до HDLC-фрейминга. `BytesReceived` отражает байты нагрузки после HDLC-декодирования.
