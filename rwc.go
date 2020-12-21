// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
)

const ChunkSize = 65536

func NewSinkWriter(sink *ByteSink) io.WriteCloser {
	return &sinkWriter{sink}
}

type sinkWriter struct {
	sink *ByteSink
}

func (w *sinkWriter) Write(data []byte) (int, error) {
	var written int

	for len(data) > 0 {
		var block []byte

		if len(data) > ChunkSize {
			block = data[:ChunkSize]
		} else {
			block = data
		}

		_, err := w.sink.Write(block)
		if err != nil {
			return written, err
		}

		data = data[len(block):]
		written += len(block)
	}

	return written, nil
}

func (w *sinkWriter) Close() error {
	return w.sink.Close()
}

func NewSourceReader(src *ByteSource) io.Reader {
	return &srcReader{
		src: src,
	}
}

type srcReader struct {
	src *ByteSource

	buf []byte
}

func (r *srcReader) Read(data []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(data, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	more := r.src.Next(context.TODO())
	if !more {
		srcErr := r.src.Err()
		if luigi.IsEOS(srcErr) {
			return 0, io.EOF
		}

		return 0, errors.Wrap(srcErr, "error getting next block")
	}

	var err error
	r.buf, err = r.src.Bytes()
	if err != nil {
		return 0, err
	}

	return r.Read(data)
}
