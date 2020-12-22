// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"io"
	"net"
)

// Endpoint allows calling functions on the RPC peer.
//go:generate counterfeiter -o mock/endpoint.go . Endpoint
type Endpoint interface {
	// The different call types:
	Async(ctx context.Context, ret interface{}, tipe RequestEncoding, method Method, args ...interface{}) error

	Source(ctx context.Context, tipe RequestEncoding, method Method, args ...interface{}) (*ByteSource, error)
	Sink(ctx context.Context, tipe RequestEncoding, method Method, args ...interface{}) (*ByteSink, error)
	Duplex(ctx context.Context, tipe RequestEncoding, method Method, args ...interface{}) (*ByteSource, *ByteSink, error)

	// Terminate wraps up the RPC session
	Terminate() error

	// Remote returns the network address of the remote
	Remote() net.Addr
}

type ByteSinker interface {
	io.WriteCloser

	// sometimes we want to close a query early before it is drained
	// (this sends a EndErr packet back )
	Cancel(error)
}
