package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	rnspipe "github.com/x3ps/go-rns-pipe"
)

var (
	framesRead          atomic.Int64
	framesWritten       atomic.Int64
	frameErrors         atomic.Int64
	lastFrameLatencyNs  atomic.Int64
)

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "frames_read_total %d\n", framesRead.Load())
	fmt.Fprintf(w, "frames_written_total %d\n", framesWritten.Load())
	fmt.Fprintf(w, "frame_errors_total %d\n", frameErrors.Load())
	fmt.Fprintf(w, "last_frame_latency_ns %d\n", lastFrameLatencyNs.Load())
}

func runShimMode() {
	decoder := rnspipe.NewDecoder(1064, 64)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for pkt := range decoder.Packets() {
			fmt.Println(hex.EncodeToString(pkt))
		}
	}()

	_, _ = io.Copy(decoder, os.Stdin)
	decoder.Close()
	<-done
}

func main() {
	shimMode := flag.Bool("shim-mode", false, "HDLC integrity test: decode stdin frames, print hex to stdout")
	metricsAddr := flag.String("metrics-addr", ":9100", "Prometheus metrics listen address")
	flag.Parse()

	if *shimMode {
		runShimMode()
		return
	}

	iface := rnspipe.New(rnspipe.Config{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
	})

	iface.OnSend(func(pkt []byte) error {
		t0 := time.Now().UnixNano()
		fmt.Fprintf(os.Stderr, "%d frame_read len=%d\n", t0, len(pkt))
		framesRead.Add(1)

		if err := iface.Receive(pkt); err != nil {
			frameErrors.Add(1)
			log.Printf("receive error: %v", err)
			return err
		}

		latency := time.Now().UnixNano() - t0
		lastFrameLatencyNs.Store(latency)
		framesWritten.Add(1)
		return nil
	})

	iface.OnStatus(func(online bool) {
		if online {
			fmt.Fprintln(os.Stderr, "interface online")
		} else {
			fmt.Fprintln(os.Stderr, "interface offline")
		}
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metricsHandler)
	srv := &http.Server{Addr: *metricsAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := iface.Start(ctx); err != nil {
		log.Printf("interface stopped: %v", err)
	}

	_ = srv.Close()
}
