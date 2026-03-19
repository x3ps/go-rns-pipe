"""E2E: link lifecycle — teardown status, rapid cycling, link-level packet burst."""

import struct
import threading
import time

import pytest
import RNS
import RNS.Channel

from conftest import EchoMessage, establish_link, wait_until

pytestmark = pytest.mark.e2e


def test_link_teardown_status(rns_client, reflector_hashes):
    """Establish link → verify ACTIVE → teardown → poll until CLOSED."""
    link = establish_link(reflector_hashes)
    assert link.status == RNS.Link.ACTIVE

    link.teardown()

    wait_until(
        lambda: link.status == RNS.Link.CLOSED,
        timeout=30,
        desc="link CLOSED after teardown",
    )
    assert link.status == RNS.Link.CLOSED


def test_rapid_link_cycle(rns_client, reflector_hashes):
    """5 iterations: establish → channel echo → teardown → CLOSED."""
    for i in range(5):
        link = establish_link(reflector_hashes)
        assert link.status == RNS.Link.ACTIVE, f"iter {i}: link not ACTIVE"

        channel = link.get_channel()
        channel.register_message_type(EchoMessage)

        received = []
        done = threading.Event()

        def on_message(message):
            received.append(message.data)
            done.set()

        channel.add_message_handler(on_message)

        payload = f"cycle-{i}".encode()
        channel.send(EchoMessage(payload))

        done.wait(timeout=15.0)
        assert len(received) == 1, f"iter {i}: no echo response"
        assert received[0] == payload, f"iter {i}: payload mismatch"

        link.teardown()

        wait_until(
            lambda: link.status == RNS.Link.CLOSED,
            timeout=30,
            desc=f"iter {i}: link CLOSED",
        )


def test_link_packet_burst(rns_client, reflector_hashes):
    """50 link-level packets sent without exceptions through UDP pipe."""
    link = establish_link(reflector_hashes)
    errors = []

    for i in range(50):
        payload = struct.pack(">qi", time.time_ns(), i) + b"\x00" * 100
        try:
            pkt = RNS.Packet(link, payload)
            pkt.send()
        except Exception as e:
            errors.append((i, str(e)))

    assert len(errors) == 0, f"Link-packet send errors: {errors}"

    link.teardown()
