---
title: TCP-транспорт
weight: 2
---

Директория `examples/tcp/` содержит готовый к производственному использованию TCP-транспорт (`rns-tcp-iface`), который проксирует трафик HDLC/pipe на TCP-пиры.

## Архитектура

```
rnsd  ←[HDLC/pipe]→  rns-tcp-iface  ←[HDLC/TCP]→  remote peer(s)
```

HDLC-фрейминг используется на **обеих** сторонах — как в пайпе к rnsd, так и в TCP-соединениях. Обе стороны используют `Encoder` и `Decoder` из библиотеки.

## Режимы

Бинарный файл поддерживает два режима, выбираемых параметром `--mode`:

| Режим | Описание |
|-------|----------|
| `client` | Подключается к удалённому TCP-серверу, переподключается с фиксированной задержкой 5s при разрыве |
| `server` | Принимает несколько клиентов; рассылает пакеты pipe→TCP всем подключённым клиентам |

## Сборка

```bash
make build-tcp
# outputs: bin/rns-tcp-iface
```

Или напрямую через Go:

```bash
go build -o rns-tcp-iface ./examples/tcp/
```

## Флаги командной строки

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--mode` | (обязательный) | `client` или `server` |
| `--listen-addr` | `:4242` | Адрес прослушивания (режим сервера) |
| `--peer-addr` | (обязательный в client) | Удалённый TCP-адрес для подключения |
| `--name` | `TCPInterface` | Имя интерфейса, передаваемое в RNS |
| `--mtu` | `500` | MTU RNS-пакетов в байтах |
| `--reconnect-delay` | `5s` | Базовая задержка переподключения (режим client) |
| `--log-level` | `info` | Уровень логирования: `debug`/`info`/`warn`/`error` |

## Переменные окружения

| Переменная | Эквивалент флага |
|------------|----------------|
| `RNS_TCP_MODE` | `--mode` |
| `RNS_TCP_NAME` | `--name` |
| `RNS_TCP_LISTEN_ADDR` | `--listen-addr` |
| `RNS_TCP_PEER_ADDR` | `--peer-addr` |

Флаги командной строки имеют приоритет над переменными окружения.

## Использование

**Режим client** (подключение к удалённому RNS-узлу):

```bash
rns-tcp-iface --mode client --peer-addr remote.host:4242 --name TCPClient
```

**Режим server** (приём подключений от удалённых узлов):

```bash
rns-tcp-iface --mode server --listen-addr 0.0.0.0:4242 --name TCPServer
```

## Конфигурация rnsd

```ini
[[TCPBridge]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/rns-tcp-iface --mode client --peer-addr remote.host:4242 --name TCPBridge
```

## Особенности реализации

### Порядок запуска

Бинарный файл использует канал `ready` для гарантии регистрации `OnSend` до того, как `Start` начнёт читать stdin:

```go
ready := make(chan struct{})

go func() {
    // runClient/runServer registers OnSend then closes ready
    errc <- runClient(ctx, cfg, iface, logger, ready)
}()

<-ready           // OnSend is registered; safe to start
go func() { errc <- iface.Start(ctx) }()
```

### Параметры TCP-сокета

Соответствуют значениям по умолчанию `TCPInterface.py`:

```go
conn.SetNoDelay(true)            // TCP_NODELAY
conn.SetKeepAlive(true)          // SO_KEEPALIVE
conn.SetKeepAlivePeriod(5*time.Second) // TCP_KEEPIDLE=5s
```

На Linux также устанавливает `TCP_KEEPINTVL=2s`, `TCP_KEEPCNT=12`, `TCP_USER_TIMEOUT=24s`.

### Аппаратный MTU

Декодер TCP использует `HW_MTU = 262144` (как в `TCPInterface.py`), что больше, чем `HWMTU = 1064` на стороне пайпа:

```go
const tcpHWMTU = 262144
decoder := rnspipe.NewDecoder(tcpHWMTU, 64)
```

### Дедлайны записи

Дедлайн записи в 5s предотвращает блокировку цикла рассылки медленными клиентами:

```go
conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
conn.Write(frame)
```

## Совместимость протокола

Фрейминг TCP идентичен Python `TCPInterface.py`:
- HDLC-фрейминг: `FLAG=0x7E`, `ESC=0x7D`, `ESC_MASK=0x20`
- Без рукопожатия при подключении — сразу сырой HDLC
- `TCP_NODELAY` включён

Это означает, что `rns-tcp-iface` в режиме client может подключаться к Python `TCPServerInterface`, и наоборот.
