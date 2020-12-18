// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/karrick/bufpool"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

type HandleOption func(*rpc)

func WithContext(ctx context.Context) HandleOption {
	return func(r *rpc) {
		r.serveCtx = ctx
	}
}

// WithRemoteAddr also sets the remote address the endpoint is connected to.
// ie if the packer tunnels through something which can't see the address.
func WithRemoteAddr(addr net.Addr) HandleOption {
	return func(r *rpc) {
		r.remote = addr
	}
}

// WithLogger let's you overwrite the stderr logger
func WithLogger(l log.Logger) HandleOption {
	return func(r *rpc) {
		r.logger = l
	}
}

func WithIsServer(yes bool) HandleOption {
	return func(r *rpc) {
		r.isServer = yes
	}
}

// IsServer tells you if the passed endpoint is in the server-role or not.
// i.e.: Did I call the remote: yes.
// Was I called by the remote: no.
// Q: don't want to extend Endpoint interface?
func IsServer(edp Endpoint) bool {
	rpc, ok := edp.(*rpc)
	if !ok {
		panic(fmt.Sprintf("not an *rpc: %T", edp))
	}

	return rpc.isServer
}

// Handle handles the connection of the packer using the specified handler.
func Handle(pkr *Packer, handler Handler, opts ...HandleOption) Endpoint {
	r := &rpc{
		pkr:  pkr,
		reqs: make(map[int32]*Request),
		root: handler,
	}

	for _, o := range opts {
		o(r)
	}
	// defaults
	if r.logger == nil {
		logger := log.NewLogfmtLogger(os.Stderr)
		logger = level.NewFilter(logger, level.AllowInfo()) // only log info and above
		r.logger = log.With(logger, "ts", log.DefaultTimestampUTC, "unit", "muxrpc")
	}

	if r.remote != nil {
		// TODO: retract remote address
		r.logger = log.With(r.logger, "remote", r.remote.String())
	}

	if r.serveCtx == nil {
		r.serveCtx = context.Background()
	}

	bp, err := bufpool.NewChanPool()
	if err != nil {
		panic(err)
	}
	r.bpool = bp

	// we need to be able to cancel in any case
	r.serveCtx, r.cancel = context.WithCancel(r.serveCtx)

	go handler.HandleConnect(r.serveCtx, r)

	return r
}

// no args should be handled as empty array not args: null
func marshalCallArgs(args []interface{}) ([]byte, error) {
	var argData []byte
	if len(args) == 0 {
		argData = []byte("[]")
	} else {
		var err error
		argData, err = json.Marshal(args)
		if err != nil {
			return nil, errors.Wrap(err, "error marshaling request arguments")
		}
	}
	return argData, nil
}

// Async does an aync call on the remote.
func (r *rpc) Async(ctx context.Context, ret interface{}, method Method, args ...interface{}) error {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return err
	}

	req := &Request{
		Type: "async",

		source: newByteSource(ctx, r.bpool),
		sink:   newByteSink(ctx, r.pkr.w),

		Method:  method,
		RawArgs: argData,
	}
	req.Stream = req.source.AsStream()

	if err := r.Do(ctx, req); err != nil {
		return fmt.Errorf("muxrpc: error sending request: %w", err)
	}

	if !req.source.Next(ctx) {
		return req.source.Err()
	}

	rd, done, err := req.source.Reader()
	if err != nil {
		return err
	}
	defer done()

	// hmm.. we might need to poke at the flag of the muxrpc packet here, too
	// because you can still transmit a string literal as JSON (then it will have quotes and maybe escape some characters differentnly)
	switch tv := ret.(type) {
	case *string:
		var bs []byte
		bs, err = ioutil.ReadAll(rd)
		if err != nil {
			return fmt.Errorf("muxrpc: error decoding json from request source: %w", err)
		}
		level.Debug(r.logger).Log("asynctype", "str", "err", err, "len", len(bs))
		*tv = string(bs)
	default:
		level.Debug(r.logger).Log("asynctype", "any")
		err = json.NewDecoder(rd).Decode(ret)
		if err != nil {
			return fmt.Errorf("muxrpc: error decoding json from request source: %w", err)
		}
	}

	return nil
}

func (r *rpc) Source(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSource, error) {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	req := &Request{
		Type: "source",

		source: newByteSource(ctx, r.bpool),
		sink:   newByteSink(ctx, r.pkr.w),

		Method:  method,
		RawArgs: argData,
	}
	req.Stream = req.source.AsStream()

	if err := r.Do(ctx, req); err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}

	return req.source, nil
}

// Sink does a sink call on the remote.
func (r *rpc) Sink(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSink, error) {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	bs := newByteSink(ctx, r.pkr.w)
	bs.pkt.Flag = bs.pkt.Flag.Set(tipe)

	req := &Request{
		Type: "sink",

		sink:   bs,
		source: newByteSource(ctx, r.bpool),

		Method:  method,
		RawArgs: argData,
	}
	req.Stream = bs.AsStream()

	if err := r.Do(ctx, req); err != nil {
		return nil, errors.Wrap(err, "error sending request")
	}

	return bs, nil
}

// Duplex does a duplex call on the remote.
func (r *rpc) Duplex(ctx context.Context, tipe codec.Flag, method Method, args ...interface{}) (*ByteSource, *ByteSink, error) {

	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, nil, err
	}

	bSrc := newByteSource(ctx, r.bpool)
	bSink := newByteSink(ctx, r.pkr.w)
	bSink.pkt.Flag = bSink.pkt.Flag.Set(tipe)

	req := &Request{
		Type: "duplex",

		source: bSrc,
		sink:   bSink,

		Method:  method,
		RawArgs: argData,

		tipe: tipe,
	}

	req.Stream = &streamDuplex{bSrc.AsStream(), bSink.AsStream()}

	if err := r.Do(ctx, req); err != nil {
		return nil, nil, errors.Wrap(err, "error sending request")
	}

	return bSrc, bSink, nil
}

// Do executes a generic call
func (r *rpc) Do(ctx context.Context, req *Request) error {
	dbg := level.Warn(r.logger)
	dbg = log.With(dbg, "call", req.Type, "method", req.Method.String())
	if req.abort == nil {
		req.abort = func() {} // noop
	}

	if req.RawArgs == nil {
		req.RawArgs = []byte("[]")
	}

	var (
		pkt codec.Packet
		err error
	)

	func() {
		r.rLock.Lock()
		defer r.rLock.Unlock()

		pkt.Flag = pkt.Flag.Set(codec.FlagJSON)
		pkt.Flag = pkt.Flag.Set(req.Type.Flags())

		pkt.Body, err = json.Marshal(req)

		r.highest++
		pkt.Req = r.highest
		r.reqs[pkt.Req] = req

		if req.sink != nil {
			req.sink.pkt = &pkt
		}

		if req.source != nil {
			req.source.hdrFlag = pkt.Flag
		}

		req.Stream.WithReq(pkt.Req)
		req.Stream.WithType(req.tipe)

		req.id = pkt.Req
	}()
	if err != nil {
		dbg.Log("event", "request create failed", "reqID", req.id, "err", err)
		return err
	}

	err = r.pkr.w.WritePacket(&pkt)
	dbg.Log("event", "request sent",
		"reqID", req.id,
		"err", err,
		"flag", pkt.Flag.String())
	return err
}
