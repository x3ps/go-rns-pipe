"""E2E: path discovery / announce propagation."""

import pytest
import RNS

from conftest import wait_until

pytestmark = pytest.mark.e2e


def test_path_discovery(rns_client, reflector_hashes):
    """has_path(dest) becomes True within 30s of request_path()."""
    dest_hash = bytes.fromhex(reflector_hashes["dest_hash"])

    if RNS.Transport.has_path(dest_hash):
        # Pre-propagated during startup — still a pass
        return

    RNS.Transport.request_path(dest_hash)
    wait_until(
        lambda: RNS.Transport.has_path(dest_hash),
        timeout=30,
        desc="path to reflector destination",
    )
