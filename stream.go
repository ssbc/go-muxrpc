package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/pkg/errors"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"
)

type Stream interface {
	luigi.Source
	luigi.Sink

	WithType(tipe interface{})
	WithReq(req int32)
}

func NewStream(src luigi.Source, sink luigi.Sink, req int32) Stream {
	return &stream{
		pktSrc:    src,
		pktSink:   sink,
		req:       req,
		closeCh:   make(chan struct{}),
		closeOnce: &sync.Once{},
	}
}

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

func (str *stream) WithType(tipe interface{}) {
	str.l.Lock()
	defer str.l.Unlock()

	str.tipe = tipe
}

func (str *stream) WithReq(req int32) {
	str.l.Lock()
	defer str.l.Unlock()

	str.req = req
}

func withCloseCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	ch := make(chan struct{})
	next := &closeCtx{
		ch:      ch,
		Context: ctx,
		once:    &sync.Once{},
	}

	go func() {
		next.l.Lock()
		defer next.l.Unlock()

		select {
		case <-ctx.Done():
		case <-ch:
			next.closed = true
		}
	}()

	return next, func() { next.once.Do(func() { close(ch) }) }
}

type closeCtx struct {
	ch <-chan struct{}
	context.Context

	l      sync.Mutex
	once   *sync.Once
	closed bool
}

func (ctx *closeCtx) Done() <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		select {
		case <-ctx.ch:
		case <-ctx.Context.Done():
		}

		close(ch)
	}()

	return ch
}

func (ctx *closeCtx) Err() error {
	select {
	case <-ctx.ch:
	case <-ctx.Context.Done():
	default:
		return nil
	}

	ctx.l.Lock()
	defer ctx.l.Unlock()

	if ctx.closed {
		return luigi.EOS{}
	}

	return ctx.Context.Err()
}

func (str *stream) Next(ctx context.Context) (interface{}, error) {
	str.l.Lock()
	defer str.l.Unlock()

	select {
	case <-str.closeCh:
		return nil, luigi.EOS{}
	default:
	}

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

	fmt.Println("got packet from stream:", vpkt, err)

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

		fmt.Println("starting unmarshal loop")
		fmt.Println("pkt.Body:", pkt.Body)
		fmt.Printf("dst(type %T): %v\n", dst, dst)
		fmt.Println("starting unmarshal loop")
		err := json.Unmarshal(pkt.Body, dst)
		fmt.Println("ending unmarshal loop")
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

func (str *stream) Close() error {
	str.closeOnce.Do(func() {
		fmt.Println("closing stream")
		pkt := buildEndPacket(str.req)
		close(str.closeCh)

		err := str.pktSink.Pour(context.TODO(), pkt)
		fmt.Println("close pour err", err)
	})

	return nil
}