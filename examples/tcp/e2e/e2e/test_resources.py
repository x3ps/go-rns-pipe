"""E2E: resource transfer micro/mini/small."""

import io

import pytest
import RNS

from conftest import wait_until

pytestmark = pytest.mark.e2e


def _establish_link(reflector_hashes):
    """Establish a link to the reflector's link destination."""
    dest_hash = bytes.fromhex(reflector_hashes["link_dest_hash"])

    if not RNS.Transport.has_path(dest_hash):
        RNS.Transport.request_path(dest_hash)
        wait_until(
            lambda: RNS.Transport.has_path(dest_hash),
            timeout=30,
            desc="path to link destination",
        )

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


def _transfer_resource(link, data, timeout):
    """Send data as a resource over the link and wait for completion."""
    resource = RNS.Resource(io.BytesIO(data), link)
    wait_until(
        lambda: resource.status >= RNS.Resource.COMPLETE,
        timeout=timeout,
        interval=0.5,
        desc=f"resource transfer ({len(data)}B)",
    )
    assert resource.status == RNS.Resource.COMPLETE, (
        f"Resource transfer failed: status={resource.status}"
    )


def test_micro_resource_128B(rns_client, reflector_hashes):
    """128B micro resource completes within 60s."""
    link = _establish_link(reflector_hashes)
    _transfer_resource(link, b"\xAA" * 128, timeout=60)
    link.teardown()


def test_mini_resource_256KB(rns_client, reflector_hashes):
    """256KB mini resource completes within 120s."""
    link = _establish_link(reflector_hashes)
    _transfer_resource(link, b"\xBB" * (256 * 1024), timeout=120)
    link.teardown()


@pytest.mark.timeout(180)
def test_small_resource_1MB(rns_client, reflector_hashes):
    """1MB small resource completes within 180s."""
    link = _establish_link(reflector_hashes)
    _transfer_resource(link, b"\xCC" * (1024 * 1024), timeout=170)
    link.teardown()
