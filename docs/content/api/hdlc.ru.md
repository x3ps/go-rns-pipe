---
title: HDLC
weight: 3
---

Файл `hdlc.go` предоставляет низкоуровневые примитивы HDLC-фрейминга. Оба типа — `Encoder` и `Decoder` — экспортированы, чтобы пользовательские транспорты могли использовать тот же фрейминг для соединений не на основе пайпа (например, TCP-пиры).

## Константы

```go
const (
    HDLCFlag    = 0x7E // Frame delimiter
    HDLCEscape  = 0x7D // Escape character
    HDLCEscMask = 0x20 // XOR mask applied to escaped bytes
)
```

Соответствуют `PipeInterface.py` строки 40–42:
```python
class HDLC:
    FLAG    = 0x7E
    ESC     = 0x7D
    ESC_MASK = 0x20
```

## Encoder

### Тип

```go
type Encoder struct{}
```

Нулевое значение `Encoder` готово к использованию — инициализация не требуется.

### `Encode`

```go
func (e *Encoder) Encode(packet []byte) []byte
```

Оборачивает `packet` в HDLC-фрейм: `FLAG + escaped(data) + FLAG`.

**Порядок экранирования (критично):** Байты ESC экранируются первыми, затем байты FLAG. Соответствует `PipeInterface.py` `HDLC.escape`.

| Входной байт | Кодируется как    |
|-------------|-------------------|
| `0x7D`      | `0x7D 0x5D`       |
| `0x7E`      | `0x7D 0x5E`       |
| другой      | без изменений     |

Возвращает новый байтовый срез. Безопасен для параллельных вызовов на одном `Encoder`.

```go
var enc rnspipe.Encoder
frame := enc.Encode([]byte{0x01, 0x7E, 0x02})
// → [0x7E, 0x01, 0x7D, 0x5E, 0x02, 0x7E]
```

## Decoder

### Тип

```go
type Decoder struct { /* unexported fields */ }
```

Потоковый декодер с состоянием. Реализует `io.Writer`, поэтому может принимать сырые байты из `io.Copy`.

Параллельные вызовы `Write` и `Close` безопасны (защищены внутренним mutex).

### `NewDecoder`

```go
func NewDecoder(hwMTU, chanSize int) *Decoder
```

Создаёт новый `Decoder`.

- `hwMTU` — фреймы длиннее этого значения молча усекаются (соответствует ограничению `HW_MTU` в `PipeInterface.py`)
- `chanSize` — ёмкость внутреннего канала пакетов

```go
decoder := rnspipe.NewDecoder(1064, 64)
```

### `Write`

```go
func (d *Decoder) Write(b []byte) (int, error)
```

Передаёт сырые байты в декодер. Декодированные полные фреймы отправляются в канал `Packets()`.

Возвращает `io.ErrClosedPipe` после вызова `Close`. Предназначен для использования с `io.Copy`:

```go
go func() {
    _, err := io.Copy(decoder, conn)
    decoder.Close()
    errCh <- err
}()
```

### `Packets`

```go
func (d *Decoder) Packets() <-chan []byte
```

Возвращает канал только для чтения, в который отправляются декодированные нагрузки пакетов. Канал закрывается после вызова `Close` и потребления всех буферизованных пакетов.

### `Close`

```go
func (d *Decoder) Close()
```

Сигнализирует о конце потока. Закрывает канал пакетов. Безопасен для многократного вызова (идемпотентен через `sync.Once`).

После `Close` `Write` возвращает `io.ErrClosedPipe`.

### `DroppedPackets`

```go
func (d *Decoder) DroppedPackets() uint64
```

Возвращает количество пакетов, отброшенных из-за заполненного канала `Packets()` (неблокирующая отправка не удалась).

## Использование в пользовательских транспортах

```go
decoder := rnspipe.NewDecoder(262144, 64) // tcpHWMTU, buffer

go func() {
    io.Copy(decoder, tcpConn)
    decoder.Close()
}()

for pkt := range decoder.Packets() {
    // forward pkt to rnsd via iface.Receive(pkt)
}
```
