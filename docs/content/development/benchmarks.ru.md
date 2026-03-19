---
title: Бенчмарки
weight: 4
---

## Запуск бенчмарков

```bash
go test -bench=. -benchmem ./...
```

Для стабильных результатов запустите несколько итераций и усредните:

```bash
go test -bench=. -benchmem -count=5 ./...
```

Запуск конкретного бенчмарка:

```bash
go test -bench=BenchmarkEncode -benchmem -count=5 ./...
```

## Доступные бенчмарки

Все бенчмарки находятся в `hdlc_test.go`:

| Бенчмарк | Что измеряется |
|----------|----------------|
| `BenchmarkEncode` | Пропускная способность HDLC-кодирования (аллокации на вызов) |
| `BenchmarkDecode` | Пропускная способность HDLC-декодирования потока |
| `BenchmarkRoundTrip` | Суммарная пропускная способность кодирования + декодирования |

## Чтение вывода

```
BenchmarkEncode-8       5000000    234 ns/op    128 B/op    1 allocs/op
BenchmarkDecode-8       8000000    178 ns/op      0 B/op    0 allocs/op
BenchmarkRoundTrip-8    3000000    412 ns/op    128 B/op    1 allocs/op
```

| Столбец | Значение |
|---------|---------|
| Суффикс `-8` | GOMAXPROCS (количество используемых логических CPU) |
| `ns/op` | Наносекунд на операцию — меньше значит быстрее |
| `B/op` | Байт кучи, аллоцированных на операцию |
| `allocs/op` | Количество аллокаций кучи на операцию |

`BenchmarkDecode` показывает `0 B/op` / `0 allocs/op`, потому что декодер использует нулевое копирование после внутренней отправки в канал — буфер аллоцируется один раз и переиспользуется. `BenchmarkEncode` аллоцирует один раз на вызов, потому что `Encoder.Encode` возвращает новый байтовый срез.

## Сравнение результатов

Установите `benchstat` для статистического сравнения:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Снимите базовые и сравниваемые результаты:

```bash
go test -bench=. -benchmem -count=10 ./... > before.txt
# make your change
go test -bench=. -benchmem -count=10 ./... > after.txt
benchstat before.txt after.txt
```

`benchstat` сообщает геометрические средние и p-значения, автоматически отфильтровывая шум.
