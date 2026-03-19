---
title: go-rns-pipe
layout: hextra-home
description: "Go implementation of the Reticulum PipeInterface protocol with HDLC framing."
---

{{< hextra/hero-badge link="https://pkg.go.dev/github.com/x3ps/go-rns-pipe" >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  <span>Go Reference ↗</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Reticulum PipeInterface&nbsp;<br class="sm:hx-block hx-hidden" />implemented in Go
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  HDLC-framed transport for the Reticulum Network Stack.&nbsp;<br class="sm:hx-block hx-hidden" />Wire-compatible with Python RNS. Zero external dependencies.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Get Started" link="getting-started" >}}
{{< hextra/hero-button text="API Reference" link="api" style="background:transparent;border:1px solid currentColor;color:inherit" >}}
</div>

<div class="hx-mt-4 hx-mb-12">

[![Go Reference](https://pkg.go.dev/badge/github.com/x3ps/go-rns-pipe.svg)](https://pkg.go.dev/github.com/x3ps/go-rns-pipe)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://github.com/x3ps/go-rns-pipe/blob/main/LICENSE)
[![CI](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml/badge.svg)](https://github.com/x3ps/go-rns-pipe/actions/workflows/ci.yml)

</div>

## What is it?

`go-rns-pipe` implements the `PipeInterface` from the [Reticulum Network Stack](https://reticulum.network/) in pure Go. It wraps any `io.Reader`/`io.Writer` pair — a TCP connection, a UDP socket, a Unix pipe — with HDLC framing identical to Python's `PipeInterface.py`. The result connects directly to `rnsd` with no configuration changes.

## Features

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Wire-Compatible"
    subtitle="Byte-exact HDLC framing matching Python RNS PipeInterface.py. Interoperates with rnsd out of the box — no patches, no flags."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(99,102,241,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Zero Dependencies"
    subtitle="Pure Go standard library only. No third-party packages. Minimal attack surface, trivial vendoring, no supply-chain risk."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-lg:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(16,185,129,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Auto-Reconnect"
    subtitle="Fixed-delay or exponential backoff with ±25% jitter. Configurable max attempts. Mirrors rnsd respawn_delay behavior exactly."
    class="hx-aspect-auto md:hx-aspect-[1.1/1] max-md:hx-min-h-[340px]"
    style="background: radial-gradient(ellipse at 50% 80%, rgba(245,158,11,0.12), transparent);"
  >}}
  {{< hextra/feature-card
    title="Context-Aware"
    subtitle="All blocking operations respect context.Context. Clean shutdown on cancellation with proper goroutine lifecycle management."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(236,72,153,0.10), transparent);"
  >}}
  {{< hextra/feature-card
    title="Thread-Safe"
    subtitle="Lock-free atomic traffic counters. Mutex-protected state transitions. Safe for concurrent Receive() calls from multiple goroutines."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(6,182,212,0.10), transparent);"
  >}}
  {{< hextra/feature-card
    title="Pluggable Transport"
    subtitle="Any io.Reader/io.Writer works. Production-ready TCP and UDP examples included. Custom transports in under 20 lines of Go."
    style="background: radial-gradient(ellipse at 50% 80%, rgba(139,92,246,0.10), transparent);"
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
Ready to connect to `rnsd`? See the [TCP transport guide](/guides/tcp-transport/) or the [rnsd integration guide](/guides/rnsd-integration/).
{{< /callout >}}
