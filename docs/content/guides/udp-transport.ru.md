---
title: UDP-транспорт
weight: 3
---

Директория `examples/udp/` содержит `rns-udp-iface`, эквивалентный Python RNS `UDPInterface`.

## Архитектура

```
rnsd  ←[HDLC/pipe]→  rns-udp-iface  ←[raw datagram]→  remote peers
```

В отличие от TCP, UDP **не использует HDLC-фрейминг** на сетевой стороне — границы датаграмм естественно разделяют пакеты.

## Сборка

```bash
make build-udp
# outputs: bin/rns-udp-iface
```

## Флаги командной строки

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--listen-addr` | `0.0.0.0:4242` | UDP-адрес для прослушивания входящих датаграмм |
| `--peer-addr` | `255.255.255.255:4242` | UDP-адрес для отправки пакетов (широковещательный или одноадресный) |
| `--name` | `UDPInterface` | Имя интерфейса, передаваемое в RNS |
| `--mtu` | `500` | MTU RNS-пакетов в байтах |
| `--log-level` | `info` | Уровень логирования: `debug`/`info`/`warn`/`error` |

## Переменные окружения

| Переменная | Эквивалент флага |
|------------|----------------|
| `RNS_UDP_LISTEN_ADDR` | `--listen-addr` |
| `RNS_UDP_PEER_ADDR` | `--peer-addr` |
| `RNS_UDP_NAME` | `--name` |
| `RNS_UDP_MTU` | `--mtu` |
| `RNS_UDP_LOG_LEVEL` | `--log-level` |

Флаги командной строки имеют приоритет над переменными окружения.

## Использование

```bash
rns-udp-iface --listen-addr 0.0.0.0:4243 --peer-addr 192.168.1.255:4243 --name UDPBridge
```

`SO_BROADCAST` всегда включён, поэтому `--peer-addr` может быть широковещательным адресом.

## Конфигурация rnsd

```ini
[[UDPBridge]]
  type = PipeInterface
  enabled = yes
  respawn_delay = 5
  command = /usr/local/bin/rns-udp-iface --listen-addr 0.0.0.0:4243 --peer-addr 192.168.1.255:4243 --name UDPBridge
```

## Особенности реализации

### Безсостоятельный дизайн

UDP — самый простой официальный пример: без состояния, без логики переподключения, без разделения client/server. Вся логика транспорта находится в `transport.go`.

### Цикл сокета

Транспорт заново открывает UDP-сокет при ошибке (как и в примере с TCP):

```go
for {
    // Resolve peer lazily — tolerates DNS not ready at startup
    peer, err := net.ResolveUDPAddr("udp", cfg.PeerAddr)

    conn, err := openUDPConn(listenAddr) // enables SO_BROADCAST

    // readLoop returns on ctx cancel or socket error
    t.readLoop(loopCtx, conn, iface)

    conn.Close()
    if loopCtx.Err() != nil {
        break
    }
    // reopen on error
}
```

### Обратный вызов OnSend

Перенаправление pipe→UDP происходит в обратном вызове `OnSend`, зарегистрированном до `iface.Start`:

```go
iface.OnSend(func(pkt []byte) error {
    if len(pkt) > cfg.MTU {
        dropped.Add(1)
        return nil
    }
    _, err := conn.WriteTo(pkt, peerAddr)
    return err
})
```

### Цикл чтения

Перенаправление UDP→pipe использует короткий дедлайн чтения для оперативного реагирования на отмену контекста:

```go
conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
n, _, err := conn.ReadFromUDP(buf)
// on timeout: continue; on error: return
iface.Receive(buf[:n])
```

### Счётчик потерь

Пакеты, отброшенные из-за превышения размера или в период переподключения, подсчитываются и логируются каждые 30 секунд.

## Совместимость протокола

Соответствует Python `UDPInterface.py`:
- Сырые датаграммы (без HDLC на сетевой стороне)
- `SO_BROADCAST` всегда включён
- Без фильтрации по IP-источнику: принимает от всех отправителей
