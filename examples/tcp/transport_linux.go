//go:build linux

package main

import (
	"log/slog"
	"net"
	"syscall"
)

const tcpUserTimeout = 18 // TCP_USER_TIMEOUT from linux/tcp.h; stable since 2.6.37

func setTCPPlatformOptions(conn *net.TCPConn, logger *slog.Logger) {
	rc, err := conn.SyscallConn()
	if err != nil {
		logger.Warn("SyscallConn failed", "error", err)
		return
	}
	var setErr error
	if ctrlErr := rc.Control(func(fd uintptr) {
		// TCP_KEEPINTVL=2s — matching TCPInterface.py keepalive_interval=2
		if e := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 2); e != nil {
			setErr = e
			return
		}
		// TCP_KEEPCNT=12 — matching TCPInterface.py keepalive_fails=12
		if e := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 12); e != nil {
			setErr = e
			return
		}
		setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, 24000)
	}); ctrlErr != nil {
		logger.Warn("rc.Control failed", "error", ctrlErr)
		return
	}
	if setErr != nil {
		logger.Warn("setsockopt failed", "error", setErr)
	}
}
