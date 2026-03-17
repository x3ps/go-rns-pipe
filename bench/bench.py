#!/usr/bin/env python3
"""
bench.py — Integration benchmark suite for go-rns-pipe.

Orchestrates five test scenarios against two Docker nodes running rnsd,
then produces plots and a Markdown report.
"""

import argparse
import csv
import math
import os
import subprocess
import socket
import statistics
import struct
import time
import sys
from pathlib import Path

try:
    import RNS
except ImportError:
    print("ERROR: RNS not installed. Run: pip install -r requirements.txt", file=sys.stderr)
    sys.exit(1)

try:
    from rich.console import Console
    from rich.table import Table
except ImportError as e:
    print(f"ERROR: missing dependency — {e}. Run: pip install -r requirements.txt", file=sys.stderr)
    sys.exit(1)

console = Console()
plt = None

# ---------------------------------------------------------------------------
# HDLC constants (must match hdlc.go)
# ---------------------------------------------------------------------------
HDLC_FLAG = 0x7E
HDLC_ESC  = 0x7D
ESC_MASK  = 0x20


def hdlc_encode(data: bytes) -> bytes:
    escaped = bytearray()
    for b in data:
        if b == HDLC_FLAG:
            escaped += bytes([HDLC_ESC, b ^ ESC_MASK])
        elif b == HDLC_ESC:
            escaped += bytes([HDLC_ESC, b ^ ESC_MASK])
        else:
            escaped.append(b)
    return bytes([HDLC_FLAG]) + bytes(escaped) + bytes([HDLC_FLAG])


def init_plotting() -> bool:
    global plt
    if plt is not None:
        return True

    try:
        import matplotlib
        matplotlib.use("Agg")
        import matplotlib.pyplot as pyplot
    except Exception as e:
        console.print(f"[yellow]Plotting disabled: {e}[/yellow]")
        return False

    plt = pyplot
    return True


def unique_values(rows: list[dict], key: str) -> list:
    return sorted({row[key] for row in rows if key in row and row[key] is not None})


def filter_rows(rows: list[dict], **conditions) -> list[dict]:
    result = []
    for row in rows:
        if all(row.get(key) == value for key, value in conditions.items()):
            result.append(row)
    return result


def values(rows: list[dict], key: str) -> list:
    return [row[key] for row in rows if row.get(key) is not None]


def median_or_nan(nums: list[float]) -> float:
    return statistics.median(nums) if nums else float("nan")


def quantile(nums: list[float], q: float) -> float:
    if not nums:
        return float("nan")
    ordered = sorted(nums)
    if len(ordered) == 1:
        return ordered[0]
    pos = (len(ordered) - 1) * q
    lower = math.floor(pos)
    upper = math.ceil(pos)
    if lower == upper:
        return ordered[lower]
    fraction = pos - lower
    return ordered[lower] * (1 - fraction) + ordered[upper] * fraction


def write_csv(rows: list[dict], path: str, fieldnames: list[str]) -> None:
    with open(path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow({name: row.get(name) for name in fieldnames})


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def wait_for_network(host: str, port: int, timeout: int = 30) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=2):
                return True
        except OSError:
            time.sleep(1)
    return False


def get_reflector_hash() -> str:
    result = subprocess.run(
        ["docker", "exec", "rns-node-b", "cat", "/tmp/reflector_hash"],
        capture_output=True, text=True, timeout=10,
    )
    if result.returncode != 0:
        raise RuntimeError(f"Could not read reflector hash: {result.stderr}")
    return result.stdout.strip()


def get_outbound_destination(dest_hash_hex: str):
    dest_hash = bytes.fromhex(dest_hash_hex)

    if not RNS.Transport.has_path(dest_hash):
        console.print("  Requesting path to reflector...")
        RNS.Transport.request_path(dest_hash)
        if not RNS.Transport.await_path(dest_hash, timeout=30):
            return None

    remote_identity = RNS.Identity.recall(dest_hash)
    if remote_identity is None:
        return None

    return RNS.Destination(
        remote_identity,
        RNS.Destination.OUT,
        RNS.Destination.SINGLE,
        "bench",
        "reflector",
    )


def configure_rns(node_a_host: str, node_a_port: int, configdir: str) -> RNS.Reticulum:
    """Start a local RNS instance connected to node-a via TCP."""
    os.makedirs(configdir, exist_ok=True)
    config_path = os.path.join(configdir, "config")
    with open(config_path, "w") as f:
        f.write(f"""[reticulum]
  enable_transport = No
  loglevel = 2

[interfaces]
  [[TCP to node-a]]
    type = TCPClientInterface
    interface_enabled = Yes
    target_host = {node_a_host}
    target_port = {node_a_port}
""")
    return RNS.Reticulum(configdir=configdir)


# ---------------------------------------------------------------------------
# Test 1 — Connectivity / announce timing
# ---------------------------------------------------------------------------

def run_test_connectivity(reticulum, dest_hash_hex: str) -> dict:
    console.print("[bold cyan]Test 1: Connectivity / announce timing[/bold cyan]")
    dest_hash = bytes.fromhex(dest_hash_hex)

    t0 = time.time()
    known = RNS.Transport.has_path(dest_hash)
    if not known:
        console.print("  Requesting path to reflector...")
        RNS.Transport.request_path(dest_hash)
        known = RNS.Transport.await_path(dest_hash, timeout=30)

    elapsed_ms = (time.time() - t0) * 1000

    result = {
        "test": "connectivity",
        "dest_found": known,
        "time_to_announce_ms": elapsed_ms if known else None,
    }

    if known:
        console.print(f"  Destination found in {elapsed_ms:.0f} ms")
    else:
        console.print("[yellow]  WARNING: destination not found within 30s[/yellow]")

    return result


# ---------------------------------------------------------------------------
# Test 2 — Packet sweep (latency + loss by size)
# ---------------------------------------------------------------------------

def run_test_packet_sweep(reticulum, dest_hash_hex: str, sizes: list, n_packets: int) -> list[dict]:
    console.print("[bold cyan]Test 2: Packet latency sweep[/bold cyan]")
    dest = get_outbound_destination(dest_hash_hex)
    if dest is None:
        console.print("[red]Could not resolve reflector destination identity[/red]")
        return []

    rows = []
    for size in sizes:
        console.print(f"  Size {size}B × {n_packets}")
        sent = 0
        received = 0

        for i in range(n_packets):
            if size < 8:
                payload = b"\x00" * size
            else:
                payload = struct.pack(">q", time.time_ns()) + b"\x00" * (size - 8)

            t_send = time.time_ns()
            try:
                pkt = RNS.Packet(dest, payload)
                receipt = pkt.send()
                sent += 1
                # Wait for delivery confirmation (not round-trip — PipeInterface is simplex here)
                deadline = time.time() + 5
                while not receipt.status == RNS.PacketReceipt.DELIVERED and time.time() < deadline:
                    time.sleep(0.05)
                latency_ms = (time.time_ns() - t_send) / 1e6
                if receipt.status == RNS.PacketReceipt.DELIVERED:
                    received += 1
                    rows.append({"size": size, "latency_ms": latency_ms, "delivered": True})
                else:
                    rows.append({"size": size, "latency_ms": latency_ms, "delivered": False})
            except Exception as e:
                console.print(f"    [red]Send error: {e}[/red]")
                rows.append({"size": size, "latency_ms": None, "delivered": False})

            time.sleep(0.05)

        loss_pct = 100 * (sent - received) / max(sent, 1)
        console.print(f"    delivered={received}/{sent} loss={loss_pct:.1f}%")

    return rows


# ---------------------------------------------------------------------------
# Test 3 — Burst
# ---------------------------------------------------------------------------

def run_test_burst(reticulum, dest_hash_hex: str, burst_size: int = 50) -> list[dict]:
    console.print("[bold cyan]Test 3: Burst throughput[/bold cyan]")
    dest = get_outbound_destination(dest_hash_hex)
    if dest is None:
        console.print("[red]Could not resolve reflector destination identity[/red]")
        return []

    payload_size = 256
    rows = []

    t_start = time.time()
    for i in range(burst_size):
        payload = struct.pack(">q", time.time_ns()) + b"\x00" * (payload_size - 8)
        t_send = time.time_ns()
        try:
            pkt = RNS.Packet(dest, payload)
            pkt.send()
            rows.append({
                "seq": i,
                "t_send_ns": t_send,
                "t_relative_ms": (time.time() - t_start) * 1000,
                "size": payload_size,
            })
        except Exception as e:
            console.print(f"    [red]Burst send error: {e}[/red]")

    elapsed = time.time() - t_start
    total_bytes = len(rows) * payload_size
    throughput_kbps = (total_bytes / 1024) / elapsed if elapsed > 0 else 0
    console.print(f"  {len(rows)}/{burst_size} packets in {elapsed:.2f}s — {throughput_kbps:.1f} KB/s")

    return rows


# ---------------------------------------------------------------------------
# Test 4 — Reconnect
# ---------------------------------------------------------------------------

def run_test_reconnect(reticulum, dest_hash_hex: str, docker: bool) -> dict:
    console.print("[bold cyan]Test 4: Reconnect after pipe-bridge restart[/bold cyan]")
    result = {
        "test": "reconnect",
        "docker": docker,
        "downtime_s": None,
        "reconnected": False,
    }

    if not docker:
        console.print("  [yellow]Skipped (--docker not set)[/yellow]")
        return result

    # Kill pipe-bridge inside rns-node-a (rnsd will respawn it via respawn_delay=2)
    t_kill = time.time()
    subprocess.run(
        ["docker", "exec", "rns-node-a", "pkill", "-f", "pipe-bridge"],
        capture_output=True,
    )
    console.print("  Killed pipe-bridge, waiting for respawn...")

    # Poll metrics endpoint to detect reconnect
    import urllib.request
    deadline = time.time() + 30
    reconnected = False
    while time.time() < deadline:
        try:
            with urllib.request.urlopen("http://localhost:9100/metrics", timeout=2) as resp:
                body = resp.read().decode()
                for line in body.splitlines():
                    if line.startswith("frames_read_total"):
                        reconnected = True
                        break
            if reconnected:
                break
        except Exception:
            pass
        time.sleep(1)

    downtime = time.time() - t_kill
    result["downtime_s"] = downtime
    result["reconnected"] = reconnected

    if reconnected:
        console.print(f"  Reconnected after {downtime:.1f}s")
    else:
        console.print("  [red]Did not reconnect within 30s[/red]")

    return result


# ---------------------------------------------------------------------------
# Test 5 — HDLC integrity
# ---------------------------------------------------------------------------

def run_test_hdlc_integrity(pipe_bridge_bin: str) -> dict:
    console.print("[bold cyan]Test 5: HDLC integrity (shim mode)[/bold cyan]")

    data = b"\x01\x02\x03\x04"
    valid_frame = hdlc_encode(data)

    # Corrupt frame: flip a byte inside payload area
    corrupted = bytearray(valid_frame)
    corrupted[2] ^= 0xFF
    corrupted_frame = bytes(corrupted)

    # Truncated frame (no closing FLAG)
    truncated_frame = bytes([HDLC_FLAG, 0x01, 0x02])

    # Empty frame
    empty_frame = bytes([HDLC_FLAG, HDLC_FLAG])

    try:
        proc = subprocess.Popen(
            [pipe_bridge_bin, "--shim-mode"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
        )
        stdin_data = corrupted_frame + truncated_frame + empty_frame + valid_frame
        stdout_data, _ = proc.communicate(stdin_data, timeout=5)
    except FileNotFoundError:
        console.print(f"  [red]pipe-bridge binary not found at {pipe_bridge_bin}[/red]")
        return {"test": "hdlc_integrity", "error": "binary not found"}
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout_data = proc.stdout.read() if proc.stdout else b""

    lines = [l.strip() for l in stdout_data.decode().splitlines() if l.strip()]
    valid_decoded = len(lines)

    # The corrupted frame decodes (HDLC doesn't have a checksum — corruption passes through)
    # The truncated frame: no closing FLAG → no packet emitted
    # Empty frame: 0 bytes payload → 1 empty packet
    # Valid frame: 1 packet
    console.print(f"  Decoded packets: {valid_decoded}")
    console.print(f"  Output lines: {lines}")

    result = {
        "test": "hdlc_integrity",
        "frames_sent": 4,
        "frames_decoded": valid_decoded,
        "output_lines": lines,
    }
    return result


# ---------------------------------------------------------------------------
# Plotting
# ---------------------------------------------------------------------------

def plot_latency_by_size(rows: list[dict], outdir: str):
    fig, ax = plt.subplots(figsize=(8, 5))
    delivered = filter_rows(rows, delivered=True)
    if not delivered:
        ax.text(0.5, 0.5, "No delivered packets", ha="center", va="center")
    else:
        sizes = unique_values(delivered, "size")
        data = [values(filter_rows(delivered, size=s), "latency_ms") for s in sizes]
        ax.boxplot(data, labels=[str(s) for s in sizes])
        ax.set_xlabel("Packet size (bytes)")
        ax.set_ylabel("Latency (ms)")
        ax.set_title("Latency by packet size")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "latency_by_size.png"), dpi=150)
    plt.close(fig)


def plot_throughput(rows: list[dict], outdir: str):
    fig, ax = plt.subplots(figsize=(8, 5))
    if not rows:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
    else:
        delivered = filter_rows(rows, delivered=True)
        sizes = unique_values(delivered, "size")
        # Approximate throughput: (n_delivered * size) / (n * 0.05s inter-packet)
        throughputs = []
        for s in sizes:
            sub = filter_rows(delivered, size=s)
            n = len(sub)
            total_time = n * 0.05 if n > 0 else 1
            kbps = (n * s / 1024) / total_time
            throughputs.append(kbps)
        ax.bar([str(s) for s in sizes], throughputs)
        ax.set_xlabel("Packet size (bytes)")
        ax.set_ylabel("Throughput (KB/s)")
        ax.set_title("Approximate throughput by packet size")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "throughput.png"), dpi=150)
    plt.close(fig)


def plot_cdf_latency(rows: list[dict], outdir: str):
    fig, ax = plt.subplots(figsize=(8, 5))
    delivered = filter_rows(rows, delivered=True)
    if not delivered:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
    else:
        for size in unique_values(delivered, "size"):
            vals = sorted(values(filter_rows(delivered, size=size), "latency_ms"))
            cdf = [(i + 1) / len(vals) for i in range(len(vals))]
            ax.plot(vals, cdf, label=f"{size}B")
        ax.set_xlabel("Latency (ms)")
        ax.set_ylabel("CDF")
        ax.set_title("Latency CDF by packet size")
        ax.legend()
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "cdf_latency.png"), dpi=150)
    plt.close(fig)


def plot_burst_timeline(rows: list[dict], outdir: str):
    fig, ax = plt.subplots(figsize=(8, 5))
    if not rows:
        ax.text(0.5, 0.5, "No data", ha="center", va="center")
    else:
        ax.scatter(values(rows, "t_relative_ms"), values(rows, "seq"), s=10)
        ax.set_xlabel("Time (ms)")
        ax.set_ylabel("Packet sequence")
        ax.set_title("Burst packet send timeline")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "burst_timeline.png"), dpi=150)
    plt.close(fig)


def plot_reconnect_timeline(result: dict, outdir: str):
    fig, ax = plt.subplots(figsize=(8, 3))
    if result.get("downtime_s") is not None:
        downtime = result["downtime_s"]
        ax.barh(["pipe-bridge"], [downtime], color="salmon", label="downtime")
        ax.set_xlabel("Seconds")
        ax.set_title(f"Reconnect downtime: {downtime:.1f}s")
    else:
        ax.text(0.5, 0.5, "Test skipped (no --docker)", ha="center", va="center")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "reconnect_timeline.png"), dpi=150)
    plt.close(fig)


def plot_hdlc_integrity(result: dict, outdir: str):
    fig, ax = plt.subplots(figsize=(6, 4))
    if "error" in result:
        ax.text(0.5, 0.5, result["error"], ha="center", va="center")
    else:
        sent = result.get("frames_sent", 0)
        decoded = result.get("frames_decoded", 0)
        ax.bar(["Sent", "Decoded"], [sent, decoded], color=["steelblue", "seagreen"])
        ax.set_title("HDLC integrity: frames sent vs decoded")
        ax.set_ylabel("Count")
    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "hdlc_integrity.png"), dpi=150)
    plt.close(fig)


def plot_summary(sweep_rows: list[dict], burst_rows: list[dict],
                 conn_result: dict, reconnect_result: dict,
                 hdlc_result: dict, outdir: str):
    fig, axes = plt.subplots(2, 3, figsize=(15, 9))
    fig.suptitle("go-rns-pipe Benchmark Summary", fontsize=14)

    # [0,0] connectivity
    ax = axes[0, 0]
    ms = conn_result.get("time_to_announce_ms")
    if ms is not None:
        ax.bar(["announce"], [ms], color="steelblue")
        ax.set_ylabel("ms")
        ax.set_title("Announce latency")
    else:
        ax.text(0.5, 0.5, "Not found", ha="center", va="center")
        ax.set_title("Announce latency")

    # [0,1] latency boxplot
    ax = axes[0, 1]
    if sweep_rows:
        delivered = filter_rows(sweep_rows, delivered=True)
        sizes = unique_values(delivered, "size")
        data = [values(filter_rows(delivered, size=s), "latency_ms") for s in sizes]
        ax.boxplot(data, labels=[str(s) for s in sizes])
    ax.set_title("Latency by size")
    ax.set_xlabel("bytes")
    ax.set_ylabel("ms")

    # [0,2] throughput
    ax = axes[0, 2]
    if sweep_rows:
        delivered = filter_rows(sweep_rows, delivered=True)
        sizes = unique_values(delivered, "size")
        throughputs = []
        for s in sizes:
            sub = filter_rows(delivered, size=s)
            n = len(sub)
            kbps = (n * s / 1024) / (n * 0.05) if n > 0 else 0
            throughputs.append(kbps)
        ax.bar([str(s) for s in sizes], throughputs, color="seagreen")
    ax.set_title("Throughput (KB/s)")
    ax.set_xlabel("bytes")

    # [1,0] burst
    ax = axes[1, 0]
    if burst_rows:
        ax.scatter(values(burst_rows, "t_relative_ms"), values(burst_rows, "seq"), s=8)
    ax.set_title("Burst timeline")
    ax.set_xlabel("ms")
    ax.set_ylabel("seq")

    # [1,1] reconnect
    ax = axes[1, 1]
    dt = reconnect_result.get("downtime_s")
    if dt is not None:
        ax.bar(["downtime"], [dt], color="salmon")
        ax.set_ylabel("s")
    else:
        ax.text(0.5, 0.5, "Skipped", ha="center", va="center")
    ax.set_title("Reconnect downtime")

    # [1,2] hdlc
    ax = axes[1, 2]
    if "error" not in hdlc_result:
        sent = hdlc_result.get("frames_sent", 0)
        decoded = hdlc_result.get("frames_decoded", 0)
        ax.bar(["Sent", "Decoded"], [sent, decoded], color=["steelblue", "seagreen"])
    else:
        ax.text(0.5, 0.5, hdlc_result.get("error", "error"), ha="center", va="center")
    ax.set_title("HDLC integrity")

    fig.tight_layout()
    fig.savefig(os.path.join(outdir, "summary.png"), dpi=150)
    plt.close(fig)


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

def write_report(results: dict, sweep_rows: list[dict], outdir: str):
    lines = ["# go-rns-pipe Benchmark Report", ""]
    lines.append(f"Generated: {time.strftime('%Y-%m-%d %H:%M:%S UTC', time.gmtime())}")
    lines.append("")

    conn = results.get("connectivity", {})
    ms = conn.get("time_to_announce_ms")
    lines.append("## 1. Connectivity")
    if ms is not None:
        lines.append(f"- Destination found: **yes** ({ms:.0f} ms)")
    else:
        lines.append("- Destination found: **no**")
    lines.append("")

    lines.append("## 2. Packet Latency Sweep")
    if sweep_rows:
        delivered = filter_rows(sweep_rows, delivered=True)
        for size in unique_values(sweep_rows, "size"):
            sub = values(filter_rows(delivered, size=size), "latency_ms")
            total = len(filter_rows(sweep_rows, size=size))
            lines.append(f"### {size}B")
            if sub:
                lines.append(f"- Delivered: {len(sub)}/{total}")
                lines.append(f"- Median latency: {median_or_nan(sub):.1f} ms")
                lines.append(f"- p95 latency: {quantile(sub, 0.95):.1f} ms")
            else:
                lines.append(f"- Delivered: 0/{total}")
            lines.append("")
    else:
        lines.append("No data collected.")
        lines.append("")

    burst = results.get("burst", {})
    lines.append("## 3. Burst")
    lines.append(f"- Packets sent: {burst.get('packets_sent', 'N/A')}")
    lines.append("")

    recon = results.get("reconnect", {})
    lines.append("## 4. Reconnect")
    if recon.get("docker"):
        dt = recon.get("downtime_s")
        lines.append(f"- Downtime: {dt:.1f}s" if dt else "- Not tested")
        lines.append(f"- Reconnected: {recon.get('reconnected', False)}")
    else:
        lines.append("- Skipped (--docker not set)")
    lines.append("")

    hdlc = results.get("hdlc_integrity", {})
    lines.append("## 5. HDLC Integrity")
    if "error" not in hdlc:
        lines.append(f"- Frames sent: {hdlc.get('frames_sent', 'N/A')}")
        lines.append(f"- Frames decoded: {hdlc.get('frames_decoded', 'N/A')}")
    else:
        lines.append(f"- Error: {hdlc['error']}")
    lines.append("")

    lines.append("## Plots")
    for fname in ["latency_by_size.png", "throughput.png", "cdf_latency.png",
                  "burst_timeline.png", "reconnect_timeline.png",
                  "hdlc_integrity.png", "summary.png"]:
        lines.append(f"- [{fname}]({fname})")
    lines.append("")

    report_path = os.path.join(outdir, "report.md")
    with open(report_path, "w") as f:
        f.write("\n".join(lines))
    console.print(f"[green]Report written to {report_path}[/green]")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="go-rns-pipe integration benchmark")
    parser.add_argument("--node-a", default="localhost:4242", help="node-a TCP address")
    parser.add_argument("--node-b", default="localhost:4243", help="node-b TCP address")
    parser.add_argument("--packets", type=int, default=500, help="packets per size in sweep")
    parser.add_argument("--sizes", default="64,256,465,500", help="comma-separated payload sizes")
    parser.add_argument("--output", default="./results", help="output directory")
    parser.add_argument("--no-plots", action="store_true", help="skip plot generation")
    parser.add_argument("--skip-hdlc", action="store_true", help="skip HDLC integrity test")
    parser.add_argument("--docker", action="store_true", help="enable reconnect test (requires docker)")
    args = parser.parse_args()

    outdir = args.output
    os.makedirs(outdir, exist_ok=True)

    node_a_host, node_a_port_str = args.node_a.rsplit(":", 1)
    node_a_port = int(node_a_port_str)

    sizes = [int(s) for s in args.sizes.split(",")]

    # Wait for node-a TCP
    console.print(f"[bold]Waiting for node-a at {args.node_a}...[/bold]")
    if not wait_for_network(node_a_host, node_a_port, timeout=60):
        console.print("[red]node-a not reachable after 60s. Aborting.[/red]")
        sys.exit(1)

    # Start local RNS instance
    configdir = os.path.join(outdir, "rns-config")
    console.print("[bold]Starting local RNS instance...[/bold]")
    reticulum = configure_rns(node_a_host, node_a_port, configdir)

    # Get reflector hash from node-b
    dest_hash_hex = None
    if args.docker:
        console.print("[bold]Fetching reflector destination hash from node-b...[/bold]")
        try:
            dest_hash_hex = get_reflector_hash()
            console.print(f"  Reflector hash: {dest_hash_hex}")
        except Exception as e:
            console.print(f"[yellow]Could not get reflector hash: {e}[/yellow]")

    if dest_hash_hex is None:
        # Generate a dummy hash for offline testing
        dest_hash_hex = "00" * 16
        console.print("[yellow]Using dummy destination hash (no Docker or reflector unavailable)[/yellow]")

    all_results = {}

    # Test 1: Connectivity
    conn_result = run_test_connectivity(reticulum, dest_hash_hex)
    all_results["connectivity"] = conn_result

    # Test 2: Packet sweep
    sweep_rows = run_test_packet_sweep(reticulum, dest_hash_hex, sizes, args.packets)
    write_csv(sweep_rows, os.path.join(outdir, "raw.csv"), ["size", "latency_ms", "delivered"])
    all_results["sweep"] = {"rows": len(sweep_rows)}

    # Test 3: Burst
    burst_rows = run_test_burst(reticulum, dest_hash_hex)
    all_results["burst"] = {"packets_sent": len(burst_rows)}

    # Test 4: Reconnect
    reconnect_result = run_test_reconnect(reticulum, dest_hash_hex, docker=args.docker)
    all_results["reconnect"] = reconnect_result

    # Test 5: HDLC integrity
    if args.skip_hdlc:
        hdlc_result = {"test": "hdlc_integrity", "skipped": True}
        console.print("[yellow]Test 5: HDLC integrity skipped[/yellow]")
    else:
        # Look for pipe-bridge binary
        candidates = [
            "./bench/pipe-bridge/pipe-bridge",
            "./pipe-bridge/pipe-bridge",
            "pipe-bridge",
        ]
        pipe_bridge_bin = None
        for c in candidates:
            if os.path.isfile(c):
                pipe_bridge_bin = c
                break
        if pipe_bridge_bin is None:
            console.print("[yellow]pipe-bridge binary not found, skipping HDLC test[/yellow]")
            hdlc_result = {"test": "hdlc_integrity", "error": "binary not found"}
        else:
            hdlc_result = run_test_hdlc_integrity(pipe_bridge_bin)
    all_results["hdlc_integrity"] = hdlc_result

    # Plots
    if not args.no_plots and init_plotting():
        console.print("[bold]Generating plots...[/bold]")
        plot_latency_by_size(sweep_rows, outdir)
        plot_throughput(sweep_rows, outdir)
        plot_cdf_latency(sweep_rows, outdir)
        plot_burst_timeline(burst_rows, outdir)
        plot_reconnect_timeline(reconnect_result, outdir)
        plot_hdlc_integrity(hdlc_result, outdir)
        plot_summary(sweep_rows, burst_rows, conn_result, reconnect_result, hdlc_result, outdir)
        console.print("[green]Plots written.[/green]")

    # Rich summary table
    table = Table(title="Benchmark Summary")
    table.add_column("Test", style="cyan")
    table.add_column("Result", style="green")

    ms = conn_result.get("time_to_announce_ms")
    table.add_row("1. Connectivity", f"{ms:.0f} ms" if ms else "NOT FOUND")

    delivered = filter_rows(sweep_rows, delivered=True)
    median_all = median_or_nan(values(delivered, "latency_ms"))
    table.add_row("2. Packet sweep", f"median={median_all:.1f}ms, n={len(sweep_rows)}")

    table.add_row("3. Burst", f"{len(burst_rows)} packets sent")

    dt = reconnect_result.get("downtime_s")
    table.add_row("4. Reconnect", f"{dt:.1f}s" if dt else "skipped")

    hdlc_decoded = hdlc_result.get("frames_decoded", "N/A")
    table.add_row("5. HDLC integrity", f"{hdlc_decoded} decoded")

    console.print(table)

    # Report
    write_report(all_results, sweep_rows, outdir)

    console.print(f"\n[bold green]Done. Results in {outdir}/[/bold green]")


if __name__ == "__main__":
    main()
