package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"

	"github.com/pkg/errors"
)

// Stream is a muxrpc stream for the general duplex case.
type Stream interface {
	luigi.Source
	luigi.Sink
	luigi.ErrorCloser

	// WithType tells the stream in what type JSON data should be unmarshalled into
	WithType(tipe interface{})

	// WithReq tells the stream what request number should be used for sent messages
	WithReq(req int32)
}

// NewStram creates a new Stream.
func NewStream(src luigi.Source, sink luigi.Sink, req int32, ins, outs bool) Stream {
	return &stream{
		pktSrc:    src,
		pktSink:   sink,
		req:       req,
		closeCh:   make(chan struct{}),
		closeOnce: &sync.Once{},
		inStream:  ins,
		outStream: outs,
	}
}

// stream implements the Stream.
type stream struct {
	l sync.Mutex

	pktSrc  luigi.Source
	pktSink luigi.Sink

	tipe      interface{}
	req       int32
	closeCh   chan struct{}
	closeOnce *sync.Once

	inStream, outStream bool
}

// WithType makes the stream unmarshal JSON into values of type tipe
func (str *stream) WithType(tipe interface{}) {
	str.l.Lock()
	defer str.l.Unlock()

	str.tipe = tipe
}

// WithReq makes the stream use req as request ID for outbound messages.
func (str *stream) WithReq(req int32) {
	str.l.Lock()
	defer str.l.Unlock()

	str.req = req
}

// Next returns the next incoming value on the stream
func (str *stream) Next(ctx context.Context) (interface{}, error) {
	str.l.Lock()
	defer str.l.Unlock()

	// cancellation
	ctx, cancel := withCloseCtx(ctx)
	defer cancel()
	go func() {
		select {
		case <-str.closeCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	vpkt, err := str.pktSrc.Next(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error reading from packet source")
	}

	pkt := vpkt.(*codec.Packet)

	if pkt.Flag.Get(codec.FlagJSON) {
		var (
			dst     interface{}
			ptrType bool
		)

		if str.tipe != nil {
			t := reflect.TypeOf(str.tipe)
			if t.Kind() == reflect.Ptr {
				ptrType = true
				t = t.Elem()
			}

			dst = reflect.New(t).Interface()
		} else {
			dst = &dst
			ptrType = true
		}

		err := json.Unmarshal(pkt.Body, dst)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshaling json")
		}

		if !ptrType {
			dst = reflect.ValueOf(dst).Elem().Interface()
		}

		return dst, nil
	} else if pkt.Flag.Get(codec.FlagString) {
		return string(pkt.Body), nil
	} else {
		return []byte(pkt.Body), nil
	}

	return pkt.Body, nil
}

// Pour sends a message on the stream
func (str *stream) Pour(ctx context.Context, v interface{}) error {
	var (
		pkt *codec.Packet
		err error
	)

	if body, ok := v.(codec.Body); ok {
		pkt = newRawPacket(str.outStream, str.req, body)
	} else if body, ok := v.(string); ok {
		pkt = newStringPacket(str.outStream, str.req, body)
	} else {
		pkt, err = newJSONPacket(str.outStream, str.req, v)
		if err != nil {
			return errors.Wrap(err, "error building json packet")
		}
	}

	err = str.pktSink.Pour(ctx, pkt)
	return errors.Wrap(err, "error pouring to packet sink")
}

// Close closes the stream and sends the EndErr message.
func (str *stream) Close() error {
	str.closeOnce.Do(func() {
		pkt := newEndOkayPacket(str.req)
		close(str.closeCh)

		// call in goroutine because we get called from the Serve-loop and
		// this causes trouble when used with net.Pipe(), because the stream is
		// unbuffered.  This shouldn't block too long and returns (a) when the
		// packet is sent, (b) if the connection is closed or some other error
		// occurs, which at some point will happen.
		go str.pktSink.Pour(context.TODO(), pkt)
	})

	return nil
}

// Close closes the stream and sends the EndErr message.
func (str *stream) CloseWithError(closeErr error) error {
	pkt, err := newEndErrPacket(str.req, closeErr)
	if err != nil {
		return errors.Wrap(err, "error building error packet")
	}

	str.closeOnce.Do(func() {
		// don't close the stream itself, otherwise the error will be dropped!

		// call in goroutine because we get called from the Serve-loop and
		// this causes trouble when used with net.Pipe(), because the stream is
		// unbuffered.  This shouldn't block too long and returns (a) when the
		// packet is sent, (b) if the connection is closed or some other error
		// occurs, which at some point will happen.
		go str.pktSink.Pour(context.TODO(), pkt)
	})

	return nil
}

// newRawPacket crafts a packet with a byte slice as payload
func newRawPacket(stream bool, req int32, body []byte) *codec.Packet {
	var flag codec.Flag

	if stream {
		flag = codec.FlagStream
	}

	return &codec.Packet{
		Flag: flag,
		Req:  req,
		Body: codec.Body(body),
	}
}

// newStringPacket crafts a new packet with string payload
func newStringPacket(stream bool, req int32, body string) *codec.Packet {
	var flag codec.Flag

	if stream {
		flag = codec.FlagStream
	}

	flag |= codec.FlagString

	return &codec.Packet{
		Flag: flag,
		Req:  req,
		Body: codec.Body(body),
	}
}

// newJSONPacket crafts a new packets with JSON payload
func newJSONPacket(stream bool, req int32, v interface{}) (*codec.Packet, error) {
	var flag codec.Flag

	if stream {
		flag = codec.FlagStream
	}

	flag |= codec.FlagJSON

	body, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling value")
	}

	return &codec.Packet{
		Flag: flag,
		Req:  req,
		Body: codec.Body(body),
	}, nil
}

var trueBytes = []byte{'t', 'r', 'u', 'e'}

func newEndOkayPacket(req int32) *codec.Packet {
	return &codec.Packet{
		Req:  req,
		Flag: codec.FlagJSON | codec.FlagEndErr | codec.FlagStream,
		Body: trueBytes,
	}
}

func newEndErrPacket(req int32, err error) (*codec.Packet, error) {
	body, err := json.Marshal(CallError{
		Message: err.Error(),
		Name:    "Error",
	})
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling value")
	}

	return &codec.Packet{
		Req:  req,
		Flag: codec.FlagJSON | codec.FlagEndErr | codec.FlagStream,
		Body: body,
	}, nil
}
