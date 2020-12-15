// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

type Method []string

func (m *Method) UnmarshalJSON(d []byte) error {
	var newM []string

	err := json.Unmarshal(d, &newM)
	if err != nil {
		var meth string
		err := json.Unmarshal(d, &meth)
		if err != nil {
			return errors.Wrap(err, "muxrpc/method: error decoding packet")
		}
		newM = Method{meth}
	}
	*m = newM

	return nil
}
func (m Method) String() string {
	return strings.Join(m, ".")
}

// Request assembles the state of an RPC call
type Request struct {
	// Stream is a legacy adapter for luigi-powered streams
	Stream Stream `json:"-"`

	// Method is the name of the called function
	Method Method `json:"name"`

	// Args contains the call arguments
	RawArgs json.RawMessage `json:"args"`

	// Type is the type of the call, i.e. async, sink, source or duplex
	Type CallType `json:"type"`

	// in is the sink that incoming packets are passed to
	in luigi.Sink

	// if in is nil, these funcs have to be set
	consume func(pktLen uint32, r io.Reader) error
	done    func(error)

	// luigi-less iterators
	sink   *ByteSink
	source *ByteSource

	// same as packet.Req - the numerical identifier for the stream
	id int32

	// tipe is a value that has the type of data we expect to receive.
	// This is needed for unmarshaling JSON.
	tipe interface{}

	// used to stop producing more data on this request
	// the calling sight might tell us they had enough of this stream
	abort context.CancelFunc
}

type ErrWrongStreamType struct{ ct CallType }

func (wst ErrWrongStreamType) Error() string {
	return fmt.Sprintf("muxrpc: wrong stream type: %s", wst.ct)
}

func (req *Request) GetSink() (*ByteSink, error) {
	if req.sink == nil {
		return nil, ErrWrongStreamType{req.Type}
	}
	return req.sink, nil
}

func (req *Request) GetSource() (*ByteSource, error) {
	if req.source == nil {
		return nil, ErrWrongStreamType{req.Type}
	}
	return req.source, nil
}

// Args is a legacy stub to get the unmarshaled json arguments
func (req *Request) Args() []interface{} {
	var v []interface{}
	json.Unmarshal(req.RawArgs, &v)
	return v
}

// Return is a helper that returns on an async call
func (req *Request) Return(ctx context.Context, v interface{}) error {
	if req.Type != "async" && req.Type != "sync" {
		return errors.Errorf("cannot return value on %q stream", req.Type)
	}

	b, err := json.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "error marshaling return value")
	}

	_, err = req.sink.Write(b)
	if err != nil {
		return errors.Wrap(err, "error writing return value")
	}

	return nil
}

func (req *Request) CloseWithError(cerr error) error {
	var inErr error
	if cerr == nil || luigi.IsEOS(errors.Cause(cerr)) {
		if req.in == nil {
			req.done(nil)
		} else { // legacy sink
			inErr = req.in.Close()
		}
	} else {
		if req.in == nil {
			req.done(cerr)
		} else { // legacy sink
			inErr = req.in.(luigi.ErrorCloser).CloseWithError(cerr)
		}
	}
	if inErr != nil {
		return errors.Wrap(inErr, "failed to close request input")
	}

	// we really need to make sure we shut down the streams.
	// "you can't" only applies for high-level abstractions.
	// this makes sure the resources go away.
	s, ok := req.Stream.(*stream)
	if !ok {
		return nil
	}
	err := s.doCloseWithError(cerr)
	if errors.Cause(err) == os.ErrClosed || IsSinkClosed(err) {
		return nil
	}
	return errors.Wrap(err, "muxrpc: failed to close request stream")
}

func (req *Request) Close() error {
	return req.CloseWithError(luigi.EOS{})
}

// CallType is the type of a call
type CallType string

// Flags returns the packet flags of the respective call type
func (t CallType) Flags() codec.Flag {
	switch t {
	case "source", "sink", "duplex":
		return codec.FlagStream
	default:
		return 0
	}
}
