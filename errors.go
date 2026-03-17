package rnspipe

import "errors"

var (
	// ErrNotStarted is returned when an operation is attempted on an interface
	// that has not been started yet.
	ErrNotStarted = errors.New("interface not started")

	// ErrAlreadyStarted is returned when Start is called on an already running interface.
	ErrAlreadyStarted = errors.New("interface already started")

	// ErrBufferFull is returned when the internal receive buffer is full and a
	// packet must be dropped.
	ErrBufferFull = errors.New("receive buffer full, packet dropped")

	// ErrInvalidPacket is returned when a received packet fails validation.
	ErrInvalidPacket = errors.New("invalid packet")
)
