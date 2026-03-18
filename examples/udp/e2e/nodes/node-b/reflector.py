#!/usr/bin/env python3
"""RNS echo server for test suite.

Two destinations on the same identity:
  ("test", "reflector") — plain packet echo with PROVE_ALL
  ("test", "server")    — link server: channel echo + resource accept
"""

import RNS
import RNS.Channel
import time

reticulum = RNS.Reticulum()

identity = RNS.Identity()
identity.to_file("/shared/reflector_identity")

# --- Destination 1: packet echo ---

packet_dest = RNS.Destination(
    identity,
    RNS.Destination.IN,
    RNS.Destination.SINGLE,
    "test",
    "reflector",
)
packet_dest.set_proof_strategy(RNS.Destination.PROVE_ALL)


def packet_callback(data, packet):
    print(f"Reflector received {len(data)}B packet", flush=True)


packet_dest.set_packet_callback(packet_callback)

# --- Destination 2: link server ---

link_dest = RNS.Destination(
    identity,
    RNS.Destination.IN,
    RNS.Destination.SINGLE,
    "test",
    "server",
)


# Channel echo message type — must match client-side definition
class EchoMessage(RNS.MessageBase):
    MSGTYPE = 0x0101

    def __init__(self, data=None):
        self.data = data or b""

    def pack(self):
        return self.data

    def unpack(self, raw):
        self.data = raw


# Track active links for defensive resource handling
active_links = {}


def on_link_established(link):
    print(f"Link established: {link}", flush=True)
    active_links[link] = True

    channel = link.get_channel()
    channel.register_message_type(EchoMessage)

    def on_channel_message(message):
        print(f"Channel message received: {len(message.data)}B", flush=True)
        response = EchoMessage(message.data)
        channel.send(response)

    channel.add_message_handler(on_channel_message)

    link.set_resource_strategy(RNS.Link.ACCEPT_ALL)
    link.set_resource_concluded_callback(on_resource_concluded)

    link.set_link_closed_callback(on_link_closed)


def on_resource_concluded(resource):
    if resource.status == RNS.Resource.COMPLETE:
        print(f"Resource transfer complete: {len(resource.data.read())}B", flush=True)
    else:
        print(f"Resource transfer failed: status={resource.status}", flush=True)


def on_link_closed(link):
    active_links.pop(link, None)
    print(f"Link closed: {link}", flush=True)


link_dest.set_link_established_callback(on_link_established)

# --- Announce and write hash files ---

packet_dest.announce()
link_dest.announce()

packet_hash_hex = RNS.hexrep(packet_dest.hash, delimit=False)
link_hash_hex = RNS.hexrep(link_dest.hash, delimit=False)
identity_hash_hex = RNS.hexrep(identity.hash, delimit=False)

with open("/shared/reflector_hash", "w") as f:
    f.write(packet_hash_hex)
with open("/shared/reflector_identity_hash", "w") as f:
    f.write(identity_hash_hex)
with open("/shared/reflector_link_hash", "w") as f:
    f.write(link_hash_hex)

print(f"Packet destination:  {packet_hash_hex}", flush=True)
print(f"Link destination:    {link_hash_hex}", flush=True)
print(f"Identity:            {identity_hash_hex}", flush=True)

print("Reflector ready, waiting for packets and links...", flush=True)

ANNOUNCE_INTERVAL = 12
last_announce = time.time()
while True:
    time.sleep(1)
    if time.time() - last_announce >= ANNOUNCE_INTERVAL:
        packet_dest.announce()
        link_dest.announce()
        last_announce = time.time()
        print("Reflector re-announced", flush=True)
