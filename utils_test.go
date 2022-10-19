// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/ssbc/go-muxrpc/v2/debug"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"
)

// for some reason you can't use t.Fatal // t.Error in goroutines... :-/
func serve(_ context.Context, r Server, errc chan<- error, done ...chan<- struct{}) {
	err := r.Serve()
	if err != nil && !errors.Is(err, context.Canceled) {
		errc <- fmt.Errorf("Serve() failed: %w", err)
	}
	if len(done) > 0 { // might want to use a waitGroup here instead?
		close(done[0])
	}
}

func mkCheck(errc chan<- error) func(err error) {
	return func(err error) {
		if err != nil {
			fmt.Println("chkerr:", err)
			errc <- err
		}
	}
}

func rewrap(l log.Logger, p *Packer) *Packer {
	rwc, ok := p.c.(io.ReadWriteCloser)
	if !ok {
		panic(fmt.Sprintf("expected RWC: %T", p.c))
	}

	return NewPacker(debug.Wrap(l, rwc))
}

func methodChecker(name string) func(Method) bool {
	return func(m Method) bool {
		return m.String() == name
	}
}

// similar to net.Pipe but using localhost loopback networking and not in-memory unbuffered
func loPipe(t testing.TB) (net.Conn, net.Conn) {
	r := require.New(t)

	aliceLis, err := net.Listen("tcp4", "localhost:0")
	r.NoError(err)

	var newConn = make(chan net.Conn)
	go func() {
		accepted, err := aliceLis.Accept()
		r.NoError(err)

		newConn <- accepted
	}()

	c1, err := net.Dial("tcp", aliceLis.Addr().String())
	r.NoError(err)
	c2 := <-newConn

	return c1, c2
}
