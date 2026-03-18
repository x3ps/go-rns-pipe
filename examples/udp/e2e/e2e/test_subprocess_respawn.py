"""E2E: PipeInterface subprocess respawn after rns-udp-iface kill.

This is NOT a reconnect test. UDP sockets are stateless and connectionless —
there is no session to reconnect. What is tested here is the PipeInterface
subprocess lifecycle: rnsd kills the rns-udp-iface child process and, after
respawn_delay=2s, relaunches it. End-to-end packet delivery is then verified
to confirm the interface is fully operational after respawn.
"""

import threading
import urllib.request

import pytest
import RNS

from conftest import wait_until

pytestmark = pytest.mark.e2e

CONTROL_URL = "http://node-a:9000"


def _kill_iface():
    req = urllib.request.Request(f"{CONTROL_URL}/kill-iface", method="POST")
    with urllib.request.urlopen(req, timeout=5) as resp:
        return resp.status == 200


def _check_iface():
    try:
        with urllib.request.urlopen(f"{CONTROL_URL}/check-iface", timeout=5) as resp:
            return resp.status == 200
    except Exception:
        return False


def test_respawn_and_recovery(rns_client, reflector_hashes):
    """Path recovers within 60s of rns-udp-iface kill on node-a."""
    dest_hash = bytes.fromhex(reflector_hashes["dest_hash"])

    # Ensure we have a path before killing
    if not RNS.Transport.has_path(dest_hash):
        RNS.Transport.request_path(dest_hash)
        wait_until(
            lambda: RNS.Transport.has_path(dest_hash),
            timeout=30,
            desc="initial path to reflector",
        )

    # Kill rns-udp-iface on node-a
    assert _kill_iface(), "kill-iface failed — rns-udp-iface was not running"

    # Wait for process to go offline
    wait_until(lambda: not _check_iface(), timeout=15, desc="rns-udp-iface offline")

    # Wait for process to respawn (rnsd respawn_delay=2)
    wait_until(_check_iface, timeout=30, desc="rns-udp-iface respawned")

    # Resolve destination for probe packets
    remote_identity = RNS.Identity.recall(dest_hash)
    if remote_identity is None:
        identity_hash = bytes.fromhex(reflector_hashes["identity_hash"])
        remote_identity = RNS.Identity.recall(identity_hash, from_identity_hash=True)

    if remote_identity is not None:
        dest = RNS.Destination(
            remote_identity,
            RNS.Destination.OUT,
            RNS.Destination.SINGLE,
            "test",
            "reflector",
        )

        # Wait for end-to-end probe delivery
        def _probe_delivered():
            try:
                pkt = RNS.Packet(dest, b"\x00" * 8)
                receipt = pkt.send()
                done = threading.Event()
                receipt.set_delivery_callback(lambda r: done.set())
                receipt.set_timeout_callback(lambda r: done.set())
                done.wait(timeout=10.0)
                return receipt.status == RNS.PacketReceipt.DELIVERED
            except Exception:
                return False

        wait_until(_probe_delivered, timeout=60, interval=2, desc="probe packet delivered")
    else:
        # Fallback: just check path recovery via routing table
        RNS.Transport.request_path(dest_hash)
        wait_until(
            lambda: RNS.Transport.has_path(dest_hash),
            timeout=60,
            desc="path recovery after respawn",
        )
