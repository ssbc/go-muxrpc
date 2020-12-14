// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"io"
	"net"

	"go.cryptoscope.co/muxrpc/codec"
)

// Endpoint allows calling functions on the RPC peer.
//go:generate counterfeiter -o mock/endpoint.go . Endpoint
type Endpoint interface {
	// The different call types:
	Async(ctx context.Context, ret interface{}, method Method, args ...interface{}) error

	Source(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSource, error)
	Sink(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSink, error)
	Duplex(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSource, *ByteSink, error)

	// Do allows general calls
	Do(ctx context.Context, req *Request) error

	// Terminate wraps up the RPC session
	Terminate() error

	// Remote returns the network address of the remote
	Remote() net.Addr
}

// ByteSourcer is inspired by sql.Rows but without the Scan(), it just reads plain []bytes, one per muxrpc packet.
type ByteSourcer interface {
	Next(context.Context) bool // blocks until there are new muxrpc frames for this stream

	// instead of returning an (un)marshaled object
	// we just give access to the received []byte contained in the muxrpc body
	Bytes() io.Reader

	// when processing fails or the context was canceled
	Err() error

	// sometimes we want to close a query early before it is drained
	// (this sends a EndErr packet back )
	Cancel(error)
}

// ByteSinker is inspired by sql.Rows but without the Scan(), it just reads plain []bytes, one per muxrpc packet.
type ByteSinker interface {
	io.WriteCloser

	// sometimes we want to close a query early before it is drained
	// (this sends a EndErr packet back )
	Cancel(error)
}
