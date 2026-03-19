---
title: Benchmarks
weight: 4
description: "Benchmark results and instructions for running performance tests."
---

## Running Benchmarks

```bash
go test -bench=. -benchmem ./...
```

For stable results, run multiple iterations and average them:

```bash
go test -bench=. -benchmem -count=5 ./...
```

Target a specific benchmark:

```bash
go test -bench=BenchmarkEncode -benchmem -count=5 ./...
```

## Available Benchmarks

All benchmarks are in `hdlc_test.go`:

| Benchmark | What it measures |
|-----------|-----------------|
| `BenchmarkEncode` | HDLC frame encoding throughput (allocations per call) |
| `BenchmarkDecode` | HDLC stream decoding throughput |
| `BenchmarkRoundTrip` | Encode + Decode combined throughput |

## Reading Output

```
BenchmarkEncode-8       5000000    234 ns/op    128 B/op    1 allocs/op
BenchmarkDecode-8       8000000    178 ns/op      0 B/op    0 allocs/op
BenchmarkRoundTrip-8    3000000    412 ns/op    128 B/op    1 allocs/op
```

| Column | Meaning |
|--------|---------|
| `-8` suffix | GOMAXPROCS (number of logical CPUs used) |
| `ns/op` | Nanoseconds per operation — lower is faster |
| `B/op` | Heap bytes allocated per operation |
| `allocs/op` | Number of heap allocations per operation |

`BenchmarkDecode` shows `0 B/op` / `0 allocs/op` because the decoder is zero-copy after the internal channel send — the buffer is allocated once and reused. `BenchmarkEncode` allocates once per call because `Encoder.Encode` returns a new byte slice.

## Comparing Results

Install `benchstat` for statistical comparison:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Capture baseline and comparison runs:

```bash
go test -bench=. -benchmem -count=10 ./... > before.txt
# make your change
go test -bench=. -benchmem -count=10 ./... > after.txt
benchstat before.txt after.txt
```

`benchstat` reports geometric mean and p-values, filtering out noise automatically.
