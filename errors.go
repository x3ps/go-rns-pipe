package rnspipe

import "errors"

var (
	// ErrNotStarted is returned when an operation is attempted on an interface
	// that has not been started yet.
	ErrNotStarted = errors.New("interface not started")

	// ErrAlreadyStarted is returned when Start is called on an already running interface.
	ErrAlreadyStarted = errors.New("interface already started")

	// ErrMaxReconnectAttemptsReached is returned by Start when all reconnect
	// attempts are exhausted (MaxReconnectAttempts > 0).
	ErrMaxReconnectAttemptsReached = errors.New("max reconnect attempts reached")

	// ErrOffline is returned by Receive when the interface is started but currently
	// offline (e.g. during a reconnect window between subprocess respawns).
	ErrOffline = errors.New("interface offline")

	// ErrPipeClosed is returned by Start when stdin reaches clean EOF and
	// ExitOnEOF is true. It signals that rnsd closed the pipe intentionally
	// and the process should exit (rnsd will respawn via respawn_delay).
	ErrPipeClosed = errors.New("pipe closed by remote")

	// ErrNoHandler is returned by Start when OnSend has not been registered.
	// The handler must be set before Start to avoid silent packet loss.
	ErrNoHandler = errors.New("OnSend handler not registered")
)
