#!/usr/bin/env python3
"""
pipe_bridge.py — HDLC loopback bridge for RNS PipeInterface benchmarking.

Spawned by rnsd as the PipeInterface command. Reads HDLC-framed packets from
stdin, decodes them, and echoes them re-encoded back to stdout. Logs per-frame
nanosecond timestamps to stderr for latency analysis.

HDLC constants match hdlc.go verbatim:
  HDLC_FLAG     = 0x7E
  HDLC_ESC      = 0x7D
  HDLC_ESC_MASK = 0x20
"""

import sys
import signal
import time

HDLC_FLAG = 0x7E
HDLC_ESC = 0x7D
HDLC_ESC_MASK = 0x20


def hdlc_encode(data: bytes) -> bytes:
    out = bytearray([HDLC_FLAG])
    for b in data:
        if b == HDLC_FLAG or b == HDLC_ESC:
            out.append(HDLC_ESC)
            out.append(b ^ HDLC_ESC_MASK)
        else:
            out.append(b)
    out.append(HDLC_FLAG)
    return bytes(out)


def hdlc_decode_frames(data: bytes) -> list:
    """Decode all complete HDLC frames from a bytes buffer. Used for testing."""
    frames = []
    buf = bytearray()
    in_frame = False
    escape = False

    for b in data:
        if b == HDLC_FLAG:
            if in_frame:
                frames.append(bytes(buf))
            buf = bytearray()
            in_frame = True
            escape = False
        elif not in_frame:
            continue
        elif b == HDLC_ESC:
            escape = True
        else:
            if escape:
                buf.append(b ^ HDLC_ESC_MASK)
                escape = False
            else:
                buf.append(b)

    return frames


def main():
    def _sigterm(_sig, _frame):
        sys.stdout.buffer.flush()
        sys.exit(0)

    signal.signal(signal.SIGTERM, _sigterm)

    stdin = sys.stdin.buffer
    stdout = sys.stdout.buffer

    buf = bytearray()
    in_frame = False
    escape = False

    while True:
        byte = stdin.read(1)
        if not byte:
            # EOF — rnsd will respawn after respawn_delay
            stdout.flush()
            sys.exit(0)

        b = byte[0]

        if b == HDLC_FLAG:
            if in_frame:
                # Complete frame received
                ts_ns = time.time_ns()
                frame_data = bytes(buf)
                sys.stderr.write(f"{ts_ns} {len(frame_data)}\n")
                sys.stderr.flush()
                stdout.write(hdlc_encode(frame_data))
                stdout.flush()
            # Reset state for next frame
            buf = bytearray()
            in_frame = True
            escape = False
        elif not in_frame:
            continue
        elif b == HDLC_ESC:
            escape = True
        else:
            if escape:
                buf.append(b ^ HDLC_ESC_MASK)
                escape = False
            else:
                buf.append(b)


if __name__ == "__main__":
    main()
