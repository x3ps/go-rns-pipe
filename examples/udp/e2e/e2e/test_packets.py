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

from conftest import ensure_has_path, wait_until

pytestmark = pytest.mark.e2e


def _resolve_destination(reflector_hashes):
    """Resolve the outbound packet destination."""
    dest_hash = bytes.fromhex(reflector_hashes["dest_hash"])

    ensure_has_path(dest_hash)

    remote_identity = RNS.Identity.recall(dest_hash)
    if remote_identity is None:
        identity_hash = bytes.fromhex(reflector_hashes["identity_hash"])
        remote_identity = RNS.Identity.recall(identity_hash, from_identity_hash=True)

    assert remote_identity is not None, "Could not resolve reflector identity"

    return RNS.Destination(
        remote_identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        "test",
        "reflector",
    )


@pytest.mark.parametrize("size", [64, 256])
def test_packet_delivery(rns_client, reflector_hashes, size):
    """Packet delivery receipt == DELIVERED within 10s."""
    dest = _resolve_destination(reflector_hashes)

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
    dest = _resolve_destination(reflector_hashes)
    errors = []

    for i in range(50):
        payload = struct.pack(">q", time.time_ns()) + b"\x00" * 248
        try:
            pkt = RNS.Packet(dest, payload)
            pkt.send()
        except Exception as e:
            errors.append((i, str(e)))

    assert len(errors) == 0, f"Send errors: {errors}"
