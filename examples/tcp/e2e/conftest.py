"""Shared fixtures for the test suite."""
import time
from pathlib import Path

import pytest
import RNS
import RNS.Channel


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def wait_until(condition, timeout=30.0, interval=0.2, desc="condition"):
    """Poll condition() until True; raises AssertionError on timeout."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return
        time.sleep(interval)
    raise AssertionError(f"Timed out after {timeout}s waiting for: {desc}")


# ---------------------------------------------------------------------------
# Channel echo message type (must match reflector.py)
# ---------------------------------------------------------------------------

class EchoMessage(RNS.MessageBase):
    MSGTYPE = 0x0101

    def __init__(self, data=None):
        self.data = data or b""

    def pack(self):
        return self.data

    def unpack(self, raw):
        self.data = raw


# ---------------------------------------------------------------------------
# Session-scoped fixtures
# ---------------------------------------------------------------------------

@pytest.fixture(scope="session")
def reflector_hashes():
    """Wait for hash files on the shared volume and return dict."""
    shared = Path("/shared")
    files = {
        "dest_hash": shared / "reflector_hash",
        "identity_hash": shared / "reflector_identity_hash",
        "link_dest_hash": shared / "reflector_link_hash",
    }
    hashes = {}

    def _all_available():
        for key, path in files.items():
            if key in hashes:
                continue
            if path.exists():
                content = path.read_text().strip()
                if content:
                    hashes[key] = content
        return len(hashes) == len(files)

    wait_until(_all_available, timeout=60, interval=2, desc="reflector hash files")
    return hashes


@pytest.fixture
def rns_client(tmp_path):
    """Start RNS instance via TCPClientInterface to node-a:4243. Fresh per test."""
    config_dir = tmp_path / "rns-config"
    config_dir.mkdir()

    config_path = config_dir / "config"
    config_path.write_text("""[reticulum]
  enable_transport = No
  share_instance = No
  loglevel = 2

[interfaces]
  [[TCP to node-a]]
    type = TCPClientInterface
    enabled = yes
    target_host = node-a
    target_port = 4243
""")
    reticulum = RNS.Reticulum(configdir=str(config_dir))

    if reticulum.is_connected_to_shared_instance:
        raise RuntimeError(
            "RNS connected to a shared instance instead of configured interfaces."
        )

    # Wait for TCP interface to come online
    def _iface_online():
        for iface in RNS.Transport.interfaces:
            if iface.name == "TCP to node-a" and iface.online:
                return True
        return False

    wait_until(_iface_online, timeout=20, desc="TCP interface online")
    yield reticulum
