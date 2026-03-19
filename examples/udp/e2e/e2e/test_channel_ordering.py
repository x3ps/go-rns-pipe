"""E2E: channel message ordering and large payload."""

import struct
import threading
import time

import pytest
import RNS
import RNS.Channel

from conftest import EchoMessage, establish_link

pytestmark = pytest.mark.e2e


def test_channel_fifo_ordering(rns_client, reflector_hashes):
    """20 numbered messages echoed back in FIFO order."""
    link = establish_link(reflector_hashes)

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    lock = threading.Lock()
    all_done = threading.Event()
    count = 20

    def on_message(message):
        with lock:
            received.append(message.data)
            if len(received) == count:
                all_done.set()

    channel.add_message_handler(on_message)

    payloads = [struct.pack(">I", i) + f"msg-{i}".encode() for i in range(count)]
    for p in payloads:
        channel.send(EchoMessage(p))
        time.sleep(0.05)

    all_done.wait(timeout=30.0)

    with lock:
        assert len(received) == count, f"Expected {count} echoes, got {len(received)}"
        for i, (sent, got) in enumerate(zip(payloads, received)):
            assert sent == got, f"Order mismatch at index {i}: sent={sent!r}, got={got!r}"

    link.teardown()


def test_channel_large_payload(rns_client, reflector_hashes):
    """Single EchoMessage at ~MDU (380B) round-trips correctly."""
    link = establish_link(reflector_hashes)

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    done = threading.Event()

    def on_message(message):
        received.append(message.data)
        done.set()

    channel.add_message_handler(on_message)

    payload = bytes(range(256)) + bytes(range(124))  # 380 bytes
    channel.send(EchoMessage(payload))

    done.wait(timeout=15.0)
    assert len(received) == 1, "Did not receive echo response"
    assert received[0] == payload, (
        f"Payload mismatch: got {len(received[0])}B, want {len(payload)}B"
    )

    link.teardown()
