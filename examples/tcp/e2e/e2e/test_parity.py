"""E2E parity: byte-level fidelity, MTU boundaries, burst delivery, bidirectional."""

import random
import struct
import threading
import time

import pytest
import RNS
import RNS.Channel

from conftest import EchoMessage, establish_link, resolve_packet_dest, wait_until

pytestmark = pytest.mark.e2e


# ---------------------------------------------------------------------------
# Module-scoped fixtures (reused across tests to avoid repeated link setup)
# ---------------------------------------------------------------------------

@pytest.fixture(scope="module")
def reflector_link(rns_client, reflector_hashes):
    """Module-scoped link with channel support."""
    link = establish_link(reflector_hashes)
    yield link
    link.teardown()


@pytest.fixture(scope="module")
def packet_dest(rns_client, reflector_hashes):
    """Module-scoped packet destination."""
    return resolve_packet_dest(reflector_hashes)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

def test_allbytes_channel_echo(rns_client, reflector_hashes):
    """All 256 byte values survive round-trip through HDLC + RNS channel."""
    link = establish_link(reflector_hashes)

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    done = threading.Event()

    def on_message(message):
        received.append(message.data)
        done.set()

    channel.add_message_handler(on_message)

    payload = bytes(range(256))
    channel.send(EchoMessage(payload))

    done.wait(timeout=15.0)
    assert len(received) == 1, "Did not receive echo response"
    assert received[0] == payload, (
        f"Byte mismatch: got {len(received[0])} bytes, want 256"
    )
    link.teardown()


@pytest.mark.parametrize("seed", range(20))
def test_random_payload_echo(reflector_link, seed):
    """Random payload survives channel echo round-trip (20 seeds)."""
    channel = reflector_link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    done = threading.Event()

    def on_message(message):
        received.append(message.data)
        done.set()

    channel.add_message_handler(on_message)

    rng = random.Random(seed)
    size = rng.randint(1, 380)
    payload = bytes(rng.getrandbits(8) for _ in range(size))

    channel.send(EchoMessage(payload))

    done.wait(timeout=15.0)
    assert len(received) == 1, f"seed={seed}: no echo response"
    assert received[0] == payload, (
        f"seed={seed}: payload mismatch (got {len(received[0])}B, want {size}B)"
    )


@pytest.mark.parametrize("size", [1, 100, 200, 300, 383])
def test_rns_mtu_boundary(packet_dest, size):
    """Packet delivery at various sizes up to conservative MTU boundary."""
    payload = struct.pack(">q", time.time_ns()) + b"\xAA" * (size - 8) if size >= 8 else b"\xAA" * size

    pkt = RNS.Packet(packet_dest, payload)
    receipt = pkt.send()

    done = threading.Event()
    receipt.set_delivery_callback(lambda r: done.set())
    receipt.set_timeout_callback(lambda r: done.set())
    done.wait(timeout=15.0)

    assert receipt.status == RNS.PacketReceipt.DELIVERED, (
        f"Packet {size}B not delivered: status={receipt.status}"
    )


def test_burst_100_with_receipts(packet_dest):
    """100 packets all reach DELIVERED status."""
    receipts = []

    for i in range(100):
        payload = struct.pack(">qi", time.time_ns(), i) + b"\x00" * 244
        pkt = RNS.Packet(packet_dest, payload)
        receipt = pkt.send()
        receipts.append(receipt)

    def all_settled():
        return all(
            r.status in (RNS.PacketReceipt.DELIVERED, RNS.PacketReceipt.FAILED)
            for r in receipts
        )

    wait_until(all_settled, timeout=60, interval=0.5, desc="all 100 receipts settled")

    delivered = sum(1 for r in receipts if r.status == RNS.PacketReceipt.DELIVERED)
    assert delivered == 100, (
        f"Only {delivered}/100 packets delivered"
    )


def test_allbytes_packet_delivery(packet_dest):
    """All 256 byte values survive round-trip via packet destination through HDLC."""
    payload = bytes(range(256))

    pkt = RNS.Packet(packet_dest, payload)
    receipt = pkt.send()

    done = threading.Event()
    receipt.set_delivery_callback(lambda r: done.set())
    receipt.set_timeout_callback(lambda r: done.set())
    done.wait(timeout=15.0)

    assert receipt.status == RNS.PacketReceipt.DELIVERED, (
        f"All-bytes packet not delivered: status={receipt.status}"
    )


def test_bidirectional_simultaneous(rns_client, reflector_hashes):
    """Both directions flow through the same HDLC pipe concurrently."""
    link = establish_link(reflector_hashes)

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    lock = threading.Lock()
    all_done = threading.Event()
    count = 10

    def on_message(message):
        with lock:
            received.append(message.data)
            if len(received) == count:
                all_done.set()

    channel.add_message_handler(on_message)

    # Send messages from a background thread with spacing.
    payloads = [f"bidir-{i}".encode() for i in range(count)]

    def sender():
        for p in payloads:
            channel.send(EchoMessage(p))
            time.sleep(0.1)

    t = threading.Thread(target=sender, daemon=True)
    t.start()

    all_done.wait(timeout=30.0)
    t.join(timeout=5.0)

    with lock:
        assert len(received) == count, (
            f"Expected {count} echoes, got {len(received)}"
        )
        for p in payloads:
            assert p in received, f"Missing echo for {p!r}"

    link.teardown()
