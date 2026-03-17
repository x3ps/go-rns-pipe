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
)
