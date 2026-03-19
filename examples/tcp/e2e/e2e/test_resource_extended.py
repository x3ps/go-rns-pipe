"""E2E: extended resource transfers — 5MB, metadata, varied sizes."""

import os

import pytest
import RNS

from conftest import establish_link, wait_until

pytestmark = pytest.mark.e2e


def _transfer_resource(link, data, timeout, metadata=None):
    """Send data as a resource over the link and wait for completion."""
    resource = RNS.Resource(data, link, metadata=metadata)
    wait_until(
        lambda: resource.status >= RNS.Resource.COMPLETE,
        timeout=timeout,
        interval=0.5,
        desc=f"resource transfer ({len(data)}B)",
    )
    assert resource.status == RNS.Resource.COMPLETE, (
        f"Resource transfer failed: status={resource.status}"
    )


@pytest.mark.timeout(360)
def test_medium_resource_5MB(rns_client, reflector_hashes):
    """5MB resource completes within 360s."""
    link = establish_link(reflector_hashes)
    data = os.urandom(5 * 1024 * 1024)
    _transfer_resource(link, data, timeout=350)
    link.teardown()


def test_resource_with_metadata(rns_client, reflector_hashes):
    """1KB resource with metadata dict completes successfully."""
    link = establish_link(reflector_hashes)
    data = os.urandom(1024)
    metadata = {"filename": "test.bin", "type": "binary", "seq": 42}
    _transfer_resource(link, data, timeout=60, metadata=metadata)
    link.teardown()


@pytest.mark.parametrize("size", [1, 64, 512, 4096, 32768])
def test_resource_varied_sizes(rns_client, reflector_hashes, size):
    """Resource transfer at various sizes from 1B to 32KB."""
    link = establish_link(reflector_hashes)
    data = os.urandom(size)
    _transfer_resource(link, data, timeout=60)
    link.teardown()
