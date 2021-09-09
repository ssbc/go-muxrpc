// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"log"
	"net"
)

//go:generate counterfeiter -o fakeendpoint_test.go . Endpoint

// Endpoint allows calling functions on the RPC peer.
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

// HasMethod returns true if an endpoint supports a specific method
func HasMethod(edp Endpoint, m Method) bool {
	rpc, ok := edp.(*rpc)
	if !ok {
		log.Printf("warning: %T is not a *rpc", edp)
		return false
	}

	_, doesHandle := rpc.manifest.Handled(m)
	return doesHandle
}
