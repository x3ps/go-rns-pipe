"""E2E: link establishment + channel round-trip."""

import threading

import pytest
import RNS
import RNS.Channel

from conftest import EchoMessage, ensure_has_path, wait_until

pytestmark = pytest.mark.e2e


def _establish_link(reflector_hashes):
    """Establish a link to the reflector's link destination."""
    dest_hash = bytes.fromhex(reflector_hashes["link_dest_hash"])

    ensure_has_path(dest_hash)

    remote_identity = RNS.Identity.recall(dest_hash)
    if remote_identity is None:
        identity_hash = bytes.fromhex(reflector_hashes["identity_hash"])
        remote_identity = RNS.Identity.recall(identity_hash, from_identity_hash=True)

    assert remote_identity is not None, "Could not resolve link destination identity"

    dest = RNS.Destination(
        remote_identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        "test",
        "server",
    )

    link = RNS.Link(dest)

    wait_until(
        lambda: link.status == RNS.Link.ACTIVE,
        timeout=30,
        desc="link ACTIVE",
    )

    return link


def test_link_establishment(rns_client, reflector_hashes):
    """Link status == ACTIVE within 30s."""
    link = _establish_link(reflector_hashes)
    assert link.status == RNS.Link.ACTIVE
    link.teardown()


def test_channel_round_trip(rns_client, reflector_hashes):
    """Echo message received back within 15s."""
    link = _establish_link(reflector_hashes)

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    received = []
    done = threading.Event()

    def on_message(message):
        received.append(message.data)
        done.set()

    channel.add_message_handler(on_message)

    test_data = b"hello from test_channel_round_trip"
    msg = EchoMessage(test_data)
    channel.send(msg)

    done.wait(timeout=15.0)
    assert len(received) == 1, "Did not receive echo response"
    assert received[0] == test_data
    link.teardown()
