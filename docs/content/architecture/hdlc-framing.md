---
title: HDLC Framing
weight: 1
description: "HDLC framing scheme used by go-rns-pipe, identical to Python PipeInterface.py."
---

`go-rns-pipe` uses a simplified HDLC framing scheme identical to Python `PipeInterface.py`. Understanding the wire format is essential when building custom transports or debugging packet loss.

## Wire Format

```
┌──────┬────────────────────────────┬──────┐
│ 0x7E │  escaped payload bytes...  │ 0x7E │
└──────┴────────────────────────────┴──────┘
  FLAG         data bytes              FLAG
```

Each frame:
1. Starts with `FLAG` (`0x7E`)
2. Contains the payload with special bytes escaped
3. Ends with `FLAG` (`0x7E`)

## Byte Stuffing

Two bytes in the payload are escaped to avoid confusion with frame delimiters:

| Original byte | Encoded as     | Explanation              |
|---------------|----------------|--------------------------|
| `0x7D` (ESC)  | `0x7D 0x5D`    | ESC → ESC, ESC^ESC_MASK  |
| `0x7E` (FLAG) | `0x7D 0x5E`    | ESC → ESC, FLAG^ESC_MASK |

**Escape order is critical:** ESC bytes (`0x7D`) must be escaped **before** FLAG bytes (`0x7E`). This matches `PipeInterface.py` `HDLC.escape`:

```python
# PipeInterface.py lines 44–47
@staticmethod
def escape(data):
    data = data.replace(bytes([HDLC.ESC]), bytes([HDLC.ESC, HDLC.ESC ^ HDLC.ESC_MASK]))
    data = data.replace(bytes([HDLC.FLAG]), bytes([HDLC.ESC, HDLC.FLAG ^ HDLC.ESC_MASK]))
    return data
```

## Example

Encoding the payload `[0x01, 0x7E, 0x7D, 0x02]`:

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

## Decoding State Machine

The decoder (`hdlc.go`) implements the same state machine as `PipeInterface.py` `readLoop` (lines 110–134):

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

## Buffer Limit

Frames larger than `HWMTU` bytes are silently truncated. The default `HWMTU=1064` matches `PipeInterface.py`:

```python
# PipeInterface.py line 72
self.HWMTU = 1064
```

## Full-Duplex Safety

- **Encoder** — stateless (`struct{}`), safe for concurrent use.
- **Decoder** — `Write` and `Close` are serialized by an internal mutex. Safe for concurrent `Write` calls from multiple goroutines.

## Empty Frames

An empty frame (two consecutive `FLAG` bytes: `7E 7E`) emits a zero-length packet, matching Python upstream which calls `process_incoming(data_buffer)` unconditionally.
