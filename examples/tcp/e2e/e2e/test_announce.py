"""E2E: path discovery / announce propagation."""

import pytest
import RNS

from conftest import ensure_has_path

pytestmark = pytest.mark.e2e


def test_path_discovery(rns_client, reflector_hashes):
    """Both reflector destinations become reachable via path discovery."""
    dest_hash = bytes.fromhex(reflector_hashes["dest_hash"])
    ensure_has_path(dest_hash)

    link_dest_hash = bytes.fromhex(reflector_hashes["link_dest_hash"])
    ensure_has_path(link_dest_hash, timeout=90)
