// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	stderr "errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// NewPacker takes an io.ReadWriteCloser and returns a Packer.
func NewPacker(rwc io.ReadWriteCloser) *Packer {
	return &Packer{
		r: codec.NewReader(rwc),
		w: codec.NewWriter(rwc),
		c: rwc,

		closing: make(chan struct{}),
	}
}

// Packer is a duplex stream that sends and receives *codec.Packet values.
// Usually wraps a network connection or stdio.
type Packer struct {
	rl sync.Mutex
	wl sync.Mutex

	r *codec.Reader
	w *codec.Writer
	c io.Closer

	cl        sync.Mutex
	closeErr  error
	closeOnce sync.Once
	closing   chan struct{}
}

// Next returns the next packet from the underlying stream.
func (pkr *Packer) NextHeader(ctx context.Context, hdr *codec.Header) error {
	pkr.rl.Lock()
	defer pkr.rl.Unlock()

	err := pkr.r.ReadHeader(hdr)
	select {
	case <-pkr.closing:
		if err != nil {
			return luigi.EOS{}
		}
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "muxrpc/packer: read packet canceled")
	default:
	}

	if err != nil {
		if errors.Cause(err) == io.EOF {
			return luigi.EOS{}
		}

		return errors.Wrap(err, "error reading packet")
	}

	hdr.Req = -hdr.Req

	return nil
}

var errSinkClosed = stderr.New("muxrpc: pour to closed sink")

// IsSinkClosed should be moved to luigi to gether with the error
func IsSinkClosed(err error) bool {
	if err == nil {
		return false
	}
	causeErr := errors.Cause(err)
	if causeErr == errSinkClosed {
		return true
	}

	if causeErr == ErrSessionTerminated {
		return true
	}

	if isAlreadyClosed(err) {
		return true
	}

	return false
}

func isAlreadyClosed(err error) bool {
	if err == nil {
		return false
	}

	causeErr := errors.Cause(err)
	if causeErr == os.ErrClosed || causeErr == io.ErrClosedPipe {
		return true
	}

	if sysErr, ok := (causeErr).(*os.PathError); ok {
		if sysErr.Err == os.ErrClosed {
			// fmt.Printf("debug: found syscall err: %T) %s\n", causeErr, causeErr)
			return true
		}
	}

	if opErr, ok := causeErr.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*os.SyscallError); ok {
			if errNo, ok := syscallErr.Err.(syscall.Errno); ok {
				if errNo == syscall.EPIPE {
					return true
				}
			}
		}
	}
	return false
}

// Close closes the packer.
func (pkr *Packer) Close() error {
	pkr.cl.Lock()
	defer pkr.cl.Unlock()
	select {
	case <-pkr.closing:
		if isAlreadyClosed(pkr.closeErr) {
			return nil
		}
		return errors.Wrap(pkr.closeErr, "packer: already closed")
	default:
	}

	var err error

	pkr.closeOnce.Do(func() {
		err = pkr.c.Close()
		close(pkr.closing)
	})
	err = errors.Wrap(err, "error closing underlying closer")
	pkr.closeErr = err
	return err
}
