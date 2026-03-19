"""E2E: resource transfer micro/mini/small."""

import pytest
import RNS

from conftest import establish_link, wait_until

pytestmark = pytest.mark.e2e


def _transfer_resource(link, data, timeout):
    """Send data as a resource over the link and wait for completion."""
    resource = RNS.Resource(data, link)
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
    link = establish_link(reflector_hashes)
    _transfer_resource(link, b"\xAA" * 128, timeout=60)
    link.teardown()


def test_mini_resource_256KB(rns_client, reflector_hashes):
    """256KB mini resource completes within 120s."""
    link = establish_link(reflector_hashes)
    _transfer_resource(link, b"\xBB" * (256 * 1024), timeout=120)
    link.teardown()


@pytest.mark.timeout(180)
def test_small_resource_1MB(rns_client, reflector_hashes):
    """1MB small resource completes within 180s."""
    link = establish_link(reflector_hashes)
    _transfer_resource(link, b"\xCC" * (1024 * 1024), timeout=170)
    link.teardown()
