"""Unit tests for bench.lib and bench.pipe_bridge — no Docker required."""

import unittest
from unittest.mock import patch, MagicMock

# Stub out RNS and rich before importing bench.lib so the module loads without them
import sys
sys.modules.setdefault("RNS", MagicMock())
sys.modules.setdefault("rich", MagicMock())
sys.modules.setdefault("rich.console", MagicMock())
sys.modules.setdefault("rich.table", MagicMock())

from bench import lib as bench_module
from bench import pipe_bridge as pipe_bridge_module


class TestWaitForReflectorHash(unittest.TestCase):
    def _make_result(self, returncode, stdout="", stderr=""):
        r = MagicMock()
        r.returncode = returncode
        r.stdout = stdout
        r.stderr = stderr
        return r

    def test_success_on_first_try(self):
        ok = self._make_result(0, stdout="deadbeef" * 4)
        with patch("bench.lib.subprocess.run", return_value=ok):
            h = bench_module.wait_for_reflector_hash(timeout=5)
        self.assertEqual(h, "deadbeef" * 4)

    def test_success_after_retry(self):
        fail = self._make_result(1, stderr="not yet")
        ok = self._make_result(0, stdout="aabbccdd" * 4)
        with patch("bench.lib.subprocess.run", side_effect=[fail, ok]):
            with patch("bench.lib.time.sleep"):
                h = bench_module.wait_for_reflector_hash(timeout=10)
        self.assertEqual(h, "aabbccdd" * 4)

    def test_timeout_raises(self):
        fail = self._make_result(1, stderr="still not ready")
        with patch("bench.lib.subprocess.run", return_value=fail):
            with patch("bench.lib.time.sleep"):
                start = 0.0
                with patch("bench.lib.time.time", side_effect=[start, start, start + 999]):
                    with self.assertRaises(RuntimeError) as ctx:
                        bench_module.wait_for_reflector_hash(timeout=1)
        self.assertIn("not available", str(ctx.exception))


class TestReconnectLogic(unittest.TestCase):
    def _make_result(self, returncode):
        r = MagicMock()
        r.returncode = returncode
        return r

    def test_skipped_without_docker(self):
        result = bench_module.run_test_reconnect(None, "00" * 16, docker=False)
        self.assertFalse(result["reconnected"])
        self.assertIsNone(result["downtime_s"])

    def test_pkill_failure_returns_early(self):
        pkill_fail = self._make_result(1)
        with patch("bench.lib.subprocess.run", return_value=pkill_fail):
            result = bench_module.run_test_reconnect(None, "00" * 16, docker=True)
        self.assertFalse(result["reconnected"])
        self.assertIsNone(result["downtime_s"])

    def test_full_offline_online_success(self):
        # pkill succeeds, pgrep returns 0 once (still running), then 1 (gone), then 0 (back)
        pkill_ok = self._make_result(0)
        still_running = self._make_result(0)
        gone = self._make_result(1)
        back = self._make_result(0)

        side_effects = [pkill_ok, still_running, gone, back]
        # 7 time.time() calls total:
        #   phase1: deadline=t[0]+15, while t[1]<deadline (still running→loop),
        #           while t[2]<deadline (gone→t_offline=t[3], break)
        #   phase2: deadline=t[4]+30, while t[5]<deadline (back→t_online=t[6], break)
        t_values = [
            100.0,  # phase1: deadline = 100.0 + 15
            100.1,  # phase1 while check iter 1 (still running, enter loop)
            100.2,  # phase1 while check iter 2 (gone, enter loop)
            100.3,  # t_offline = time.time()
            100.4,  # phase2: deadline = 100.4 + 30
            100.5,  # phase2 while check iter 1 (back, enter loop)
            100.6,  # t_online = time.time()
        ]

        with patch("bench.lib.subprocess.run", side_effect=side_effects):
            with patch("bench.lib.time.sleep"):
                with patch("bench.lib.time.time", side_effect=t_values):
                    result = bench_module.run_test_reconnect(None, "00" * 16, docker=True)

        self.assertTrue(result["reconnected"])
        self.assertIsNotNone(result["downtime_s"])
        self.assertAlmostEqual(result["downtime_s"], 0.3, places=5)

    def test_no_respawn_timeout(self):
        pkill_ok = self._make_result(0)
        gone = self._make_result(1)

        # pgrep always returns 1 after pkill (never respawns)
        with patch("bench.lib.subprocess.run", side_effect=[pkill_ok, gone] + [gone] * 100):
            with patch("bench.lib.time.sleep"):
                # 6 time.time() calls total:
                #   phase1: deadline=t[0]+15, while t[1]<deadline (gone→t_offline=t[2], break)
                #   phase2: deadline=t[3]+30, while t[4]<deadline (still gone→loop),
                #           while t[5]>deadline (past deadline, exit)
                t_values = [
                    100.0,   # phase1: deadline = 100.0 + 15
                    100.1,   # phase1 while check iter 1 (gone, enter loop)
                    100.2,   # t_offline = time.time()
                    100.3,   # phase2: deadline = 100.3 + 30
                    100.4,   # phase2 while check iter 1 (still gone, enter loop)
                    1099.0,  # phase2 while check iter 2 (past deadline)
                ]
                with patch("bench.lib.time.time", side_effect=t_values):
                    result = bench_module.run_test_reconnect(None, "00" * 16, docker=True)

        self.assertFalse(result["reconnected"])
        self.assertIsNone(result["downtime_s"])


class TestPipeBridgeHdlcDecodeFrames(unittest.TestCase):
    def test_empty_frame_is_included(self):
        # FLAG FLAG = valid empty packet
        result = pipe_bridge_module.hdlc_decode_frames(bytes([0x7E, 0x7E]))
        self.assertEqual(result, [b""])

    def test_normal_frame(self):
        from bench.lib import hdlc_encode
        encoded = hdlc_encode(b"\x01\x02\x03")
        result = pipe_bridge_module.hdlc_decode_frames(encoded)
        self.assertEqual(result, [b"\x01\x02\x03"])


class TestHdlcIntegrityEmptyFrame(unittest.TestCase):
    def test_empty_frame_is_counted(self):
        # Shim mode stdout: corrupted line, empty line (empty frame), valid line
        stdout_data = b"01fc02ff\n\ndeadbeef01020304\n"
        mock_proc = MagicMock()
        mock_proc.communicate.return_value = (stdout_data, b"")
        with patch("bench.lib.subprocess.Popen", return_value=mock_proc):
            result = bench_module.run_test_hdlc_integrity("/fake/pipe-bridge")
        self.assertEqual(result["frames_decoded"], 3)


if __name__ == "__main__":
    unittest.main()
