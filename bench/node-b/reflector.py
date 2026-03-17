#!/usr/bin/env python3
"""RNS echo server for bench suite. Reflects packets back to sender."""

import RNS
import time
import struct
import os

reticulum = RNS.Reticulum()

identity = RNS.Identity()
identity.to_file("/tmp/reflector_identity")

destination = RNS.Destination(
    identity,
    RNS.Destination.IN,
    RNS.Destination.SINGLE,
    "bench",
    "reflector",
)
destination.set_proof_strategy(RNS.Destination.PROVE_ALL)

destination.announce()

dest_hash_hex = RNS.hexrep(destination.hash, delimit=False)
with open("/tmp/reflector_hash", "w") as f:
    f.write(dest_hash_hex)

print(f"Reflector destination: {dest_hash_hex}", flush=True)


def packet_callback(data, packet):
    try:
        reply = RNS.Packet(packet.generate_proof_destination(), data)
        reply.send()
    except Exception as e:
        print(f"Reflector error: {e}", flush=True)


destination.set_packet_callback(packet_callback)

print("Reflector ready, waiting for packets...", flush=True)

while True:
    time.sleep(1)
