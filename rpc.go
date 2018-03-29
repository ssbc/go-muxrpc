package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"
)

type rpc struct {
	l sync.Mutex

	// pkr is the Sink and Source of the network connection
	pkr Packer

	// reqs is the map we keep, tracking all requests
	reqs    map[int32]*Request
	highest int32

	root Handler
}

type Handler interface {
	OnCall(ctx context.Context, req *Request)
	OnConnect(ctx context.Context, e Endpoint)
}

const bufSize = 5
const rxTimeout time.Duration = time.Millisecond

func Handle(pkr Packer, handler Handler) Endpoint {
	r := &rpc{
		pkr:  pkr,
		reqs: make(map[int32]*Request),
		root: handler,
	}

	go handler.OnConnect(context.Background(), r)
	return r
}

// Async does an aync call to the endpoint.
func (r *rpc) Async(ctx context.Context, dst interface{}, method []string, args ...interface{}) error {
	inSrc, inSink := luigi.NewPipe(luigi.WithBuffer(bufSize))

	req := &Request{
		Type: "async",
		In:   inSrc,
		in:   inSink,

		Method: method,
		Args:   args,
	}

	err := r.Do(ctx, req)
	if err != nil {
		return errors.Wrap(err, "error sending request")
	}

	v, err := req.In.Next(ctx)
	if err != nil {
		return errors.Wrap(err, "error reading response from request source")
	}

	data := v.([]byte)

	if err := json.Unmarshal(data, dst); err != nil {
		return errors.Wrapf(err, "async: Unmarshalling for method %v failed.", method)
	}
	return nil
}

func (r *rpc) Source(ctx context.Context, method []string, args ...interface{}) (luigi.Source, error) {
	inSrc, inSink := luigi.NewPipe(luigi.WithBuffer(bufSize))

	req := &Request{
		Type: "source",
		In:   inSrc,
		in:   inSink,

		Method: method,
		Args:   args,
	}

	err := r.Do(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}

	return req.In, nil
}

func (r *rpc) Sink(ctx context.Context, method []string, args ...interface{}) (luigi.Sink, error) {
	inSrc, inSink := luigi.NewPipe(luigi.WithBuffer(bufSize))

	req := &Request{
		Type: "sink",
		In:   inSrc,
		in:   inSink,

		Method: method,
		Args:   args,
	}

	err := r.Do(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}

	return req.Out, nil
}

func (r *rpc) Duplex(ctx context.Context, method []string, args ...interface{}) (luigi.Source, luigi.Sink, error) {
	inSrc, inSink := luigi.NewPipe(luigi.WithBuffer(bufSize))

	req := &Request{
		Type: "duplex",
		In:   inSrc,
		in:   inSink,

		Method: method,
		Args:   args,
	}

	err := r.Do(ctx, req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error sending request")
	}

	return req.In, req.Out, nil
}

var trueBytes = []byte{'t', 'r', 'u', 'e'}

func buildEndPacket(req int32) *codec.Packet {
	return &codec.Packet{
		Req:  req,
		Flag: codec.FlagJSON | codec.FlagEndErr | codec.FlagStream,
		Body: trueBytes,
	}
}

func (r *rpc) finish(ctx context.Context, req int32) error {
	delete(r.reqs, req)

	err := r.pkr.Pour(ctx, buildEndPacket(req))
	return errors.Wrap(err, "error pouring done message")
}

func (r *rpc) Do(ctx context.Context, req *Request) error {
	var (
		pkt codec.Packet
		err error
	)

	func() {
		r.l.Lock()
		defer r.l.Unlock()

		pkt.Flag = pkt.Flag.Set(codec.FlagJSON)
		pkt.Flag = pkt.Flag.Set(req.Type.Flags())

		pkt.Body, err = json.Marshal(req)

		pkt.Req = r.highest + 1
		r.highest = pkt.Req
		r.reqs[pkt.Req] = req

		if req.Type == "sink" || req.Type == "duplex" {
			req.Out = NewBytesPacker(r.pkr, pkt.Flag, pkt.Req)
		}

		req.pkt = &pkt
	}()
	if err != nil {
		return err
	}

	return r.pkr.Pour(ctx, &pkt)
}

func (r *rpc) ParseRequest(pkt *codec.Packet) (*Request, error) {
	var req Request

	if !pkt.Flag.Get(codec.FlagJSON) {
		return nil, errors.New("expected JSON flag")
	}

	if pkt.Req >= 0 {
		// request numbers should have been inverted by now
		return nil, errors.New("expected negative request id")
	}

	req.pkt = pkt

	err := json.Unmarshal(pkt.Body, &req)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding packet")
	}

	if req.Type == "source" || req.Type == "sink" || req.Type == "duplex" {
		inSrc, inSink := luigi.NewPipe(luigi.WithBuffer(bufSize))
		req.In = inSrc
		req.in = inSink
	}
	req.Out = NewBytesPacker(r.pkr, pkt.Flag, pkt.Req)

	return &req, nil
}

func isTrue(data []byte) bool {
	return len(data) == 4 &&
		data[0] == 't' &&
		data[1] == 'r' &&
		data[2] == 'u' &&
		data[3] == 'e'
}

func (r *rpc) Serve(ctx context.Context) (err error) {
	defer func() {
		fmt.Printf("Serve returns with err=%q\n", err)

		fmt.Println("finishing all connections - taking lock")
		r.l.Lock()
		defer r.l.Unlock()
		fmt.Println("got lock")

		for req := range r.reqs {
			r.finish(ctx, req)
		}

		fmt.Println("closing")
		r.pkr.Close()
	}()

	for {
		var v interface{}
		// read next packet from connection
		v, err = r.pkr.Next(ctx)
		fmt.Printf("pkr.Next returned %+v, %+v\n", v, err)
		if luigi.IsEOS(err) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "error reading from packer source")
		}

		pkt := v.(*codec.Packet)

		// get request from map, otherwise make new one
		req, ok := r.reqs[pkt.Req]
		if !ok {
			if pkt.Flag.Get(codec.FlagEndErr) {
				continue
			}

			req, err = r.ParseRequest(pkt)
			fmt.Printf("ParseRequest returned %+v, %+v\n", req, err)
			if err != nil {
				return errors.Wrap(err, "error parsing request")
			}
			r.reqs[pkt.Req] = req

			go r.root.OnCall(ctx, req)

			continue
		}

		// is this packet ending a stream?
		if pkt.Flag.Get(codec.FlagEndErr) {
			delete(r.reqs, pkt.Req)

			if !isTrue(pkt.Body) {
				fmt.Printf("not true: %q\n", pkt.Body)
				err = req.in.Pour(ctx, []byte(pkt.Body))
				if err != nil {
					return errors.Wrap(err, "error writing to pipe sink")
				}
			}

			if req.in != nil {
				err = req.in.Close()
				if err != nil {
					return errors.Wrap(err, "error closing pipe sink")
				}
			}

			pkt.Body = []byte{'t', 'r', 'u', 'e'}
			pkt.Req = -pkt.Req
			err = r.pkr.Pour(ctx, pkt)
			if err != nil {
				return errors.Wrap(err, "error pouring end reply to packer")
			}

			continue
		}

		// localize defer
		err = func() error {
			// pour may block so we need to time out.
			// note that you can use buffers make this less probable
			ctx, cancel := context.WithTimeout(ctx, rxTimeout)
			defer cancel()

			err := req.in.Pour(ctx, []byte(pkt.Body))
			return errors.Wrap(err, "error pouring data to handler")
		}()
		if err != nil {
			return err
		}
	}
}
