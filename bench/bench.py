#!/usr/bin/env python3
"""
bench.py — CLI entry point for the go-rns-pipe integration benchmark suite.

All test logic lives in bench.lib. This file only handles argument parsing
and top-level orchestration.
"""

import argparse
import os
import sys

# When executed as a script (python3 bench/bench.py), Python adds bench/ to
# sys.path.  Ensure the repo root is present so that 'bench' resolves as a
# package and 'bench.lib' is importable.
_repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _repo_root not in sys.path:
    sys.path.insert(0, _repo_root)

from bench.lib import (
    console,
    filter_rows,
    init_plotting,
    median_or_nan,
    plot_burst_timeline,
    plot_cdf_latency,
    plot_hdlc_integrity,
    plot_latency_by_size,
    plot_reconnect_timeline,
    plot_summary,
    plot_throughput,
    configure_rns,
    run_test_burst,
    run_test_connectivity,
    run_test_hdlc_integrity,
    run_test_packet_sweep,
    run_test_reconnect,
    values,
    wait_for_network,
    wait_for_reflector_hash,
    write_csv,
    write_report,
    Table,
)


def main():
    parser = argparse.ArgumentParser(description="go-rns-pipe integration benchmark")
    parser.add_argument("--node-a", default="localhost:4242", help="node-a TCP address")
    parser.add_argument("--packets", type=int, default=500, help="packets per size in sweep")
    parser.add_argument("--sizes", default="64,256,465,500", help="comma-separated payload sizes")
    parser.add_argument("--output", default="./results", help="output directory")
    parser.add_argument("--no-plots", action="store_true", help="skip plot generation")
    parser.add_argument("--skip-hdlc", action="store_true", help="skip HDLC integrity test")
    parser.add_argument("--docker", action="store_true",
                        help="auto-fetch reflector hash from Docker container and enable reconnect test")
    parser.add_argument("--reflector-hash", metavar="HEX",
                        help="reflector destination hash (alternative to --docker for tests 1-3)")
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

    # Determine reflector hash
    if args.docker:
        console.print("[bold]Fetching reflector destination hash from node-b...[/bold]")
        try:
            dest_hash_hex = wait_for_reflector_hash(timeout=60)
            console.print(f"  Reflector hash: {dest_hash_hex}")
        except RuntimeError as e:
            console.print(f"[red]FATAL: {e}[/red]")
            sys.exit(1)
    elif args.reflector_hash:
        dest_hash_hex = args.reflector_hash
        console.print(f"  Reflector hash: {dest_hash_hex}")
    else:
        dest_hash_hex = None
        console.print("[yellow]Tests 1–3 skipped: no --docker or --reflector-hash[/yellow]")

    all_results = {}

    # Tests 1–3: require a reflector hash
    if dest_hash_hex is None:
        conn_result = {"test": "connectivity", "skipped": True, "dest_found": False,
                       "time_to_announce_ms": None}
        sweep_rows = []
        burst_rows = []
    else:
        # Test 1: Connectivity
        conn_result = run_test_connectivity(reticulum, dest_hash_hex)

        # Test 2: Packet sweep
        sweep_rows = run_test_packet_sweep(reticulum, dest_hash_hex, sizes, args.packets)
        write_csv(sweep_rows, os.path.join(outdir, "raw.csv"), ["size", "latency_ms", "delivered"])

        # Test 3: Burst
        burst_rows = run_test_burst(reticulum, dest_hash_hex)

    all_results["connectivity"] = conn_result
    all_results["sweep"] = {"rows": len(sweep_rows)}
    all_results["burst"] = {"packets_sent": len(burst_rows)}

    # Test 4: Reconnect
    reconnect_result = run_test_reconnect(reticulum, dest_hash_hex or "00" * 16,
                                          docker=args.docker)
    all_results["reconnect"] = reconnect_result

    # Test 5: HDLC integrity
    if args.skip_hdlc:
        hdlc_result = {"test": "hdlc_integrity", "skipped": True}
        console.print("[yellow]Test 5: HDLC integrity skipped[/yellow]")
    else:
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

    if dest_hash_hex is None:
        table.add_row("1. Connectivity", "skipped")
        table.add_row("2. Packet sweep", "skipped")
        table.add_row("3. Burst", "skipped")
    else:
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
