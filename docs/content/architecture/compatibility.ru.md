---
title: Совместимость с Python RNS
weight: 3
---

`go-rns-pipe` разработан для совместимости с Python RNS на уровне протокола. На этой странице задокументированы точные соответствия и известные различия.

## Матрица совместимости

| Возможность | Python PipeInterface.py | go-rns-pipe | Примечания |
|-------------|------------------------|-------------|------------|
| HDLC FLAG | `0x7E` | `0x7E` | Идентично |
| HDLC ESC | `0x7D` | `0x7D` | Идентично |
| HDLC ESC_MASK | `0x20` | `0x20` | Идентично |
| Порядок экранирования | ESC первым, затем FLAG | ESC первым, затем FLAG | Идентично |
| HWMTU | `1064` | `1064` (по умолчанию) | Настраиваемо |
| MTU | `500` | `500` (по умолчанию) | Настраиваемо |
| Задержка переподключения | `respawn_delay=5` | `ReconnectDelay=5s` | Идентичное поведение |
| Пустые фреймы | доставляются | доставляются | Идентично |
| Без рукопожатия при подключении | ✓ | ✓ | Идентично |

## Поведенческие различия

### Стратегия переподключения

| Аспект | Python | Go |
|--------|--------|-----|
| Стратегия по умолчанию | Фиксированный `respawn_delay` | Фиксированный `ReconnectDelay` (то же) |
| Экспоненциальный откат | Недоступен | Опционально через `ExponentialBackoff=true` |
| Максимальное число попыток | Бесконечно | Настраиваемо через `MaxReconnectAttempts` |

Python PipeInterface полагается на `respawn_delay` rnsd для перезапуска дочернего процесса. `go-rns-pipe` может опционально управлять переподключением внутри без перезапуска процесса.

### Некорректные escape-последовательности

Обе реализации пропускают нераспознанные escape-последовательности без изменений:

```python
# PipeInterface.py — any byte after ESC is unescaped via XOR
byte ^= HDLC.ESC_MASK
```

```go
// go-rns-pipe — only valid sequences are remapped; others pass through as-is
switch byte_ {
case HDLCFlag ^ HDLCEscMask: // 0x5E → 0x7E
    byte_ = HDLCFlag
case HDLCEscape ^ HDLCEscMask: // 0x5D → 0x7D
    byte_ = HDLCEscape
// no default: byte_ unchanged
}
```

Примечание: Python XOR-декодирует все байты (поэтому `0x7D 0xAB` → `0x8B`); Go декодирует только две корректные последовательности. Для корректных данных результат идентичен; некорректные данные могут различаться.

### Счётчики трафика

Python `PipeInterface.py` не ведёт счётчики трафика. `go-rns-pipe` предоставляет атомарные счётчики:

```go
iface.PacketsSent()
iface.PacketsReceived()
iface.BytesSent()
iface.BytesReceived()
```

### Логирование

Python использует `RNS.log`. `go-rns-pipe` использует `log/slog` (структурированное логирование). Передайте пользовательский `*slog.Logger` через `Config.Logger`.

## Совместимость TCP

Транспорт `examples/tcp` совместим с Python `TCPInterface.py` на уровне протокола:

| Возможность | TCPInterface.py | rns-tcp-iface |
|-------------|----------------|---------------|
| Фрейминг | HDLC | HDLC |
| Рукопожатие | Нет | Нет |
| TCP_NODELAY | ✓ | ✓ |
| SO_KEEPALIVE | ✓ | ✓ |
| TCP_KEEPIDLE | 5s | 5s |
| HW_MTU | 262144 | 262144 |
| Переподключение клиента | 5s фиксировано | 5s фиксировано |

## Совместимость UDP

Транспорт `examples/udp` совместим с Python `UDPInterface.py` на уровне протокола:

| Возможность | UDPInterface.py | rns-udp-iface |
|-------------|----------------|---------------|
| Фрейминг на уровне сети | Нет (сырые датаграммы) | Нет (сырые датаграммы) |
| SO_BROADCAST | ✓ | ✓ |
| Фильтрация по источнику | Нет | Нет |
