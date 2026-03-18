//go:build !linux

package main

import (
	"log/slog"
	"net"
)

func setTCPPlatformOptions(_ *net.TCPConn, _ *slog.Logger) {} // no-op on non-Linux
