---
title: go-rns-pipe
layout: hextra-home
---

{{< hextra/hero-badge link="https://pkg.go.dev/github.com/x3ps/go-rns-pipe" >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  <span>Go Reference ↗</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Reticulum PipeInterface&nbsp;<br class="sm:hx-block hx-hidden" />реализован на Go
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  HDLC-транспорт для Reticulum Network Stack.&nbsp;<br class="sm:hx-block hx-hidden" />Совместим на уровне протокола с Python RNS. Без внешних зависимостей.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Начало работы" link="getting-started" >}}
{{< hextra/hero-button text="Справочник API" link="api" style="background:transparent;border:1px solid currentColor;color:inherit" >}}
</div>

<div class="hx-mt-4 hx-mb-12">

[![Go Reference](https://pkg.go.dev/badge/github.com/x3ps/go-rns-pipe.svg)](https://pkg.go.dev/github.com/x3ps/go-rns-pipe)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://github.com/x3ps/go-rns-pipe/blob/main/LICENSE)
[![CI](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml/badge.svg)](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml)

</div>

## Что это такое?

`go-rns-pipe` реализует `PipeInterface` из [Reticulum Network Stack](https://reticulum.network/) на чистом Go. Он оборачивает любую пару `io.Reader`/`io.Writer` — TCP-соединение, UDP-сокет, Unix-пайп — с HDLC-фреймингом, идентичным Python `PipeInterface.py`. Результат подключается непосредственно к `rnsd` без изменения конфигурации.

## Возможности

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Совместимость на уровне протокола"
    subtitle="Побайтово точный HDLC-фрейминг, соответствующий Python RNS PipeInterface.py. Работает с rnsd из коробки — без патчей и флагов."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(99,102,241,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Без зависимостей"
    subtitle="Только стандартная библиотека Go. Никаких сторонних пакетов. Минимальная поверхность атаки, простое вендоринг, без рисков цепочки поставок."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(16,185,129,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Авто-переподключение"
    subtitle="Фиксированная задержка или экспоненциальный откат с ±25% джиттером. Настраиваемое максимальное число попыток. Точно воспроизводит поведение respawn_delay в rnsd."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(245,158,11,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Поддержка Context"
    subtitle="Все блокирующие операции учитывают context.Context. Чистое завершение при отмене с правильным управлением жизненным циклом goroutine."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(236,72,153,0.10), transparent);"
  >}}
  {{< hextra/feature-card
    title="Потокобезопасность"
    subtitle="Атомарные счётчики трафика без блокировок. Mutex-защищённые переходы состояний. Безопасные параллельные вызовы Receive() из нескольких goroutine."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(6,182,212,0.10), transparent);"
  >}}
  {{< hextra/feature-card
    title="Подключаемый транспорт"
    subtitle="Подходит любая пара io.Reader/io.Writer. Готовые примеры TCP и UDP. Пользовательский транспорт — менее чем в 20 строках Go."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(139,92,246,0.10), transparent);"
  >}}
{{< /hextra/feature-grid >}}

## Быстрый старт

```bash
go get github.com/x3ps/go-rns-pipe
```

```go
package main

import (
    "context"
    "log"

    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    iface := rnspipe.New(rnspipe.Config{
        Name:      "MyInterface",
        ExitOnEOF: true,
    })

    // Called for each HDLC-decoded packet received from rnsd
    iface.OnSend(func(pkt []byte) error {
        log.Printf("RNS → app: %d bytes", len(pkt))
        return nil
    })

    iface.OnStatus(func(online bool) {
        log.Printf("interface online=%v", online)
    })

    if err := iface.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

{{< callout type="info" >}}
Готовы подключиться к `rnsd`? Смотрите [руководство по TCP-транспорту](/ru/guides/tcp-transport/) или [руководство по интеграции с rnsd](/ru/guides/rnsd-integration/).
{{< /callout >}}
