// SPDX-License-Identifier: MIT

package debug

import (
	"bytes"
	"io"
	"net"
	"sync"

	"github.com/go-kit/kit/log"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

func newLogWriter(l log.Logger) *logWriter {
	r, w := io.Pipe()

	return &logWriter{
		l:           l,
		r:           codec.NewReader(r),
		WriteCloser: w,
	}
}

type logWriter struct {
	l log.Logger
	r *codec.Reader
	io.WriteCloser
}

func (lw *logWriter) work() (func(), chan error) {
	cancel := make(chan struct{})
	errCh := make(chan error)

	go func() {
		var (
			err error
			pkt *codec.Packet
		)

		defer func() {
			errCh <- err
		}()

		for {
			select {
			case <-cancel:
				return
			default:
			}

			pkt, err = lw.r.ReadPacket()
			if err != nil {
				lw.l.Log("error", err)

				// don't send EOF over error channel, because that error is okay
				if err == io.EOF {
					err = nil
				}

				return
			}
			if pkt.Flag.Get(codec.FlagJSON) {

				lw.l.Log("req", pkt.Req, "flag", pkt.Flag, "body", bytes.Replace(pkt.Body, []byte(`"`), []byte("'"), -1))
			} else {
				lw.l.Log("req", pkt.Req, "flag", pkt.Flag, "body", pkt.Body)
			}
		}
	}()

	var closeOnce sync.Once

	return func() {
		closeOnce.Do(func() {
			close(cancel)
		})
	}, errCh
}

type closer func() error

func (c closer) Close() error { return c() }

// Wrap decodes every packet that passes through it and logs it
func Wrap(l log.Logger, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	lwIn := newLogWriter(log.With(l, "dir", "in"))
	cnclIn, errChIn := lwIn.work()

	lwOut := newLogWriter(log.With(l, "dir", "out"))
	cnclOut, errChOut := lwOut.work()

	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: io.TeeReader(rwc, lwIn),
		Writer: io.MultiWriter(rwc, lwOut),
		Closer: closer(func() error {

			cnclIn()

			cnclOut()

			var err error

			err = multierror.Append(err, lwIn.Close())

			err = multierror.Append(err, lwOut.Close())

			if errIn := <-errChIn; errors.Cause(errIn) != io.EOF && errIn != nil {
				err = multierror.Append(err, errIn)
			}

			if errOut := <-errChOut; errors.Cause(errOut) != io.EOF && errOut != nil {
				err = multierror.Append(err, errOut)
			}

			err = multierror.Append(err, rwc.Close())

			// TODO: make better multierror -.-
			if merr, ok := err.(*multierror.Error); ok {
				if merr.Len() == 0 {
					return nil
					/*
						} else {
							for _, e := range merr.Errors {
								if strings.HasSuffix(errors.Cause(e).Error(), "file already closed") {
									// ignore
									return nil
								} else {
									fmt.Printf("err:%T %#v", e, e)
									return e
								}
							}
					*/
				}
			}

			return err
		}),
	}
}

type wrappedConn struct {
	net.Conn
	rwc io.ReadWriteCloser
}

func (conn *wrappedConn) Read(data []byte) (int, error) {
	return conn.rwc.Read(data)
}

func (conn *wrappedConn) Write(data []byte) (int, error) {
	return conn.rwc.Write(data)
}

func (conn *wrappedConn) Close() error {
	return conn.rwc.Close()
}

func WrapConn(l log.Logger, conn net.Conn) net.Conn {
	return &wrappedConn{conn, Wrap(l, conn)}
}
