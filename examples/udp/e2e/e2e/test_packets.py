"""E2E: packet delivery receipts + burst.

UDP transport uses a 1064-byte MTU (rns-udp-iface default). Unlike TCP's
stream-based framing, each RNS packet maps to one UDP datagram. Within a local
Docker bridge network datagrams are not reordered or dropped, so delivery
receipts are expected to succeed. The burst test asserts only that pkt.send()
raises zero exceptions — it does not assert per-packet delivery receipts, as
UDP does not guarantee delivery.
"""

import struct
import threading
import time

import pytest
import RNS

from conftest import resolve_packet_dest, wait_until

pytestmark = pytest.mark.e2e


@pytest.mark.parametrize("size", [64, 256])
def test_packet_delivery(rns_client, reflector_hashes, size):
    """Packet delivery receipt == DELIVERED within 10s."""
    dest = resolve_packet_dest(reflector_hashes)

    if size < 8:
        payload = b"\x00" * size
    else:
        payload = struct.pack(">q", time.time_ns()) + b"\x00" * (size - 8)

    pkt = RNS.Packet(dest, payload)
    receipt = pkt.send()

    done = threading.Event()
    receipt.set_delivery_callback(lambda r: done.set())
    receipt.set_timeout_callback(lambda r: done.set())
    done.wait(timeout=10.0)

    assert receipt.status == RNS.PacketReceipt.DELIVERED, (
        f"Packet {size}B not delivered: status={receipt.status}"
    )


def test_burst_50_packets(rns_client, reflector_hashes):
    """Burst 50 packets with zero send exceptions.

    UDP does not guarantee delivery so per-packet receipts are not checked.
    This test validates only that the send path raises no exceptions under load.
    """
    dest = resolve_packet_dest(reflector_hashes)
    errors = []

    for i in range(50):
        payload = struct.pack(">q", time.time_ns()) + b"\x00" * 248
        try:
            pkt = RNS.Packet(dest, payload)
            pkt.send()
        except Exception as e:
            errors.append((i, str(e)))

    assert len(errors) == 0, f"Send errors: {errors}"
