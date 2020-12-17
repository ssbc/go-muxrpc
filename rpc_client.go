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

	"go.cryptoscope.co/muxrpc/codec"
)

// Handle handles the connection of the packer using the specified handler.
func Handle(pkr *Packer, handler Handler) Endpoint {
	return handle(pkr, handler, nil, nil)
}

// HandleWithRemote also sets the remote address the endpoint is connected to
// TODO: better passing through packer maybe?!
func HandleWithRemote(pkr *Packer, handler Handler, addr net.Addr) Endpoint {
	return handle(pkr, handler, addr, nil)
}

// HandleWithLogger same as Handle but let's you overwrite the stderr logger
func HandleWithLogger(pkr *Packer, handler Handler, logger log.Logger) Endpoint {
	return handle(pkr, handler, nil, logger)
}

func handle(pkr *Packer, handler Handler, remote net.Addr, logger log.Logger) Endpoint {
	if logger == nil {
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = level.NewFilter(logger, level.AllowInfo()) // only log info and above
		logger = log.With(logger, "ts", log.DefaultTimestampUTC, "unit", "muxrpc")
	}
	if remote == nil {
		if ra, ok := pkr.c.(interface{ RemoteAddr() net.Addr }); ok {
			remote = ra.RemoteAddr()
		}
	}
	if remote != nil {
		logger = log.With(logger, "remote", remote.String())
	}

	// TODO: rpc root context!? serve context?!
	ctx := context.TODO()

	ctx, cancel := context.WithCancel(ctx)
	r := &rpc{
		logger: logger,
		remote: remote,
		pkr:    pkr,
		reqs:   make(map[int32]*Request),
		root:   handler,

		cancel: cancel,
	}

	bp, err := bufpool.NewChanPool()
	if err != nil {
		panic(err)
	}
	r.bpool = bp

	go handler.HandleConnect(ctx, r)

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

	// rd = io.TeeReader(rd, os.Stderr)
	// hmm.. we might need to poke at the flag of the muxrpc packet here, too
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

	if err := r.Do(ctx, req); err != nil {
		return nil, nil, errors.Wrap(err, "error sending request")
	}

	return bSrc, bSink, fmt.Errorf("TODO: duplex as Stream")
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
