// Example demonstrates using go-rns-pipe as a TCP-to-RNS bridge.
//
// It connects to a TCP server and bridges traffic bidirectionally through
// the RNS PipeInterface HDLC protocol on stdin/stdout.
//
// Usage:
//
//	go run . <tcp-address>
//
// Example (connect to a TCP service on localhost:4242):
//
//	go run . localhost:4242
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <tcp-address>\n", os.Args[0])
		os.Exit(1)
	}
	addr := os.Args[1]

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("dial %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()

	// Create interface with TCP conn as stdin/stdout — HDLC frames flow
	// over the TCP connection while RNS reads/writes via os stdin/stdout.
	iface := rnspipe.New(rnspipe.Config{
		Name:   "tcp-bridge",
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
	})

	// Packets decoded from stdin (from rnsd) are forwarded to TCP.
	iface.OnSend(func(pkt []byte) error {
		_, err := conn.Write(pkt)
		return err
	})

	iface.OnStatus(func(online bool) {
		if online {
			log.Println("interface online")
		} else {
			log.Println("interface offline")
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("bridging stdin/stdout <-> %s", addr)
	if err := iface.Start(ctx); err != nil {
		log.Fatalf("interface stopped: %v", err)
	}
}
