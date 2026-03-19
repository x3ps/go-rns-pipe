---
title: go-rns-pipe
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  <span>Go 1.26+</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  RNS PipeInterface&nbsp;in Go
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  HDLC-framed transport for Reticulum Network Stack.&nbsp;<br class="sm:hx-block hx-hidden" />Zero external dependencies. Wire-compatible with Python RNS.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Get Started" link="getting-started" >}}
{{< hextra/hero-button text="API Reference" link="api" style="outline" >}}
</div>

<div class="hx-mt-6">

[![Go Reference](https://pkg.go.dev/badge/github.com/x3ps/go-rns-pipe.svg)](https://pkg.go.dev/github.com/x3ps/go-rns-pipe)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://github.com/x3ps/go-rns-pipe/blob/main/LICENSE)
[![CI](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml/badge.svg)](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml)

</div>

## Features

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Wire-Compatible"
    subtitle="Byte-exact HDLC framing matching Python RNS PipeInterface.py. Interoperates with rnsd out of the box."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
  >}}
  {{< hextra/feature-card
    title="Zero Dependencies"
    subtitle="Pure Go standard library only. No third-party packages. Minimal attack surface and simple vendoring."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
  >}}
  {{< hextra/feature-card
    title="Auto-Reconnect"
    subtitle="Fixed-delay or exponential backoff with ±25% jitter. Configurable max attempts. Matches rnsd respawn_delay behavior."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
  >}}
  {{< hextra/feature-card
    title="Context-Aware"
    subtitle="All blocking operations respect context.Context. Clean shutdown on cancellation with proper goroutine lifecycle management."
  >}}
  {{< hextra/feature-card
    title="Thread-Safe"
    subtitle="Lock-free atomic traffic counters. Mutex-protected state transitions. Safe for concurrent Receive() calls."
  >}}
  {{< hextra/feature-card
    title="Transport Examples"
    subtitle="Production-ready TCP and UDP transport examples included. Build custom transports with the simple io.Reader/io.Writer interface."
  >}}
{{< /hextra/feature-grid >}}

## Quick Start

```bash
go get github.com/x3ps/go-rns-pipe
```

```go
package main

import (
    "context"
    "log"
    "os"

    rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
    iface := rnspipe.New(rnspipe.Config{
        Name:      "MyInterface",
        ExitOnEOF: true,
    })

    // Called for each decoded RNS packet from rnsd
    iface.OnSend(func(pkt []byte) error {
        log.Printf("received %d bytes from RNS", len(pkt))
        return nil
    })

    // Called on online/offline transitions
    iface.OnStatus(func(online bool) {
        log.Printf("interface online=%v", online)
    })

    if err := iface.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```
