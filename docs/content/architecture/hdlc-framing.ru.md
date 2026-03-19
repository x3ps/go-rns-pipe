---
title: HDLC-фрейминг
weight: 1
---

`go-rns-pipe` использует упрощённую схему HDLC-фрейминга, идентичную Python `PipeInterface.py`. Понимание формата данных на уровне протокола необходимо при создании пользовательских транспортов или отладке потери пакетов.

## Формат данных

```
┌──────┬────────────────────────────┬──────┐
│ 0x7E │  escaped payload bytes...  │ 0x7E │
└──────┴────────────────────────────┴──────┘
  FLAG         data bytes              FLAG
```

Каждый фрейм:
1. Начинается с `FLAG` (`0x7E`)
2. Содержит нагрузку с экранированными специальными байтами
3. Заканчивается `FLAG` (`0x7E`)

## Байтовое экранирование

Два байта в нагрузке экранируются, чтобы избежать путаницы с разделителями фреймов:

| Исходный байт | Кодируется как | Пояснение              |
|---------------|----------------|------------------------|
| `0x7D` (ESC)  | `0x7D 0x5D`    | ESC → ESC, ESC^ESC_MASK  |
| `0x7E` (FLAG) | `0x7D 0x5E`    | ESC → ESC, FLAG^ESC_MASK |

**Порядок экранирования критичен:** байты ESC (`0x7D`) должны экранироваться **перед** байтами FLAG (`0x7E`). Соответствует `PipeInterface.py` `HDLC.escape`:

```python
# PipeInterface.py lines 44–47
@staticmethod
def escape(data):
    data = data.replace(bytes([HDLC.ESC]), bytes([HDLC.ESC, HDLC.ESC ^ HDLC.ESC_MASK]))
    data = data.replace(bytes([HDLC.FLAG]), bytes([HDLC.ESC, HDLC.FLAG ^ HDLC.ESC_MASK]))
    return data
```

## Пример

Кодирование нагрузки `[0x01, 0x7E, 0x7D, 0x02]`:

```
Input:  01  7E  7D  02
         │   │   │
         │   │   └─ ESC → 7D 5D
         │   └───── FLAG → 7D 5E
         └───────── unchanged

Encoded payload: 01  7D 5E  7D 5D  02

Frame: 7E  01  7D 5E  7D 5D  02  7E
       ↑                          ↑
      FLAG                       FLAG
```

## Конечный автомат декодирования

Декодер (`hdlc.go`) реализует тот же конечный автомат, что и `PipeInterface.py` `readLoop` (строки 110–134):

```
state: outside_frame
  0x7E → state: inside_frame, reset buffer

state: inside_frame
  0x7E → emit packet, state: outside_frame
  0x7D → state: escape_next
  other (len < HWMTU) → append to buffer

state: escape_next (inside_frame)
  0x5E → append 0x7E, state: inside_frame
  0x5D → append 0x7D, state: inside_frame
  other → append as-is (malformed; pass through matching Python behavior)
```

## Ограничение буфера

Фреймы длиннее `HWMTU` байт молча усекаются. По умолчанию `HWMTU=1064` соответствует `PipeInterface.py`:

```python
# PipeInterface.py line 72
self.HWMTU = 1064
```

## Полнодуплексная безопасность

- **Encoder** — без состояния (`struct{}`), безопасен для параллельного использования.
- **Decoder** — `Write` и `Close` сериализованы внутренним mutex. Безопасен для параллельных вызовов `Write` из нескольких goroutine.

## Пустые фреймы

Пустой фрейм (два последовательных байта `FLAG`: `7E 7E`) генерирует пакет нулевой длины, как и в Python, где `process_incoming(data_buffer)` вызывается безусловно.
