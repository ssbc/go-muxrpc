// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// Async does an aync call on the remote.
func (r *rpc) Async(ctx context.Context, ret interface{}, re RequestEncoding, method Method, args ...interface{}) error {
	_, ok := r.manifest.Handled(method)
	if !ok {
		return fmt.Errorf("muxrpc(%s): method is not on their manifest", method)
	}

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

	req.sink.pkt.Flag, err = re.asCodecFlag()
	if err != nil {
		return err
	}

	if err := r.start(ctx, req); err != nil {
		return fmt.Errorf("muxrpc(%s): error sending request: %w", method, err)
	}

	if !req.source.Next(ctx) {
		err := req.source.Err()
		return fmt.Errorf("muxrpc(%s): did not receive data for request: %w", method, err)
	}

	processEntry := func(rd io.Reader) error {
		switch tv := ret.(type) {
		case *[]byte:
			if re != TypeBinary {
				return fmt.Errorf("unexpected requst encoding, need TypeBinary got %v", re)
			}
			var bs []byte
			bs, err = ioutil.ReadAll(rd)
			if err != nil {
				return fmt.Errorf("error decoding json from request source: %w", err)
			}
			*tv = bs

		case *string:
			if re != TypeString {
				return fmt.Errorf("unexpected requst encoding, need TypeString got %v", re)
			}
			var bs []byte
			bs, err = ioutil.ReadAll(rd)
			if err != nil {
				return fmt.Errorf("error decoding json from request source: %w", err)
			}
			level.Debug(r.logger).Log("asynctype", "str", "err", err, "len", len(bs))
			*tv = string(bs)

		default:
			if re != TypeJSON {
				return fmt.Errorf("unexpected requst encoding, need TypeJSON got %v for %T", re, tv)
			}
			level.Debug(r.logger).Log("asynctype", "any")
			err = json.NewDecoder(rd).Decode(ret)
			if err != nil {
				return fmt.Errorf("error decoding json from request source: %w", err)
			}
		}
		return nil
	}

	if err := req.source.Reader(processEntry); err != nil {
		srcErr := req.source.Err()
		return fmt.Errorf("muxrpc(%s): async call failed: %s (%w)", method, err, srcErr)
	}

	return nil
}

func (r *rpc) Source(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSource, error) {
	_, ok := r.manifest.Handled(method)
	if !ok {
		return nil, fmt.Errorf("muxrpc(%s): method is not on their manifest", method)
	}

	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	encFlag, err := re.asCodecFlag()
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
	req.sink.pkt.Flag = req.sink.pkt.Flag.Set(encFlag)

	req.Stream = req.source.AsStream()

	if err := r.start(ctx, req); err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	return req.source, nil
}

// Sink does a sink call on the remote.
func (r *rpc) Sink(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSink, error) {
	_, ok := r.manifest.Handled(method)
	if !ok {
		return nil, fmt.Errorf("muxrpc(%s): method is not on their manifest", method)
	}

	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	encFlag, err := re.asCodecFlag()
	if err != nil {
		return nil, err
	}

	req := &Request{
		Type: "sink",

		sink:   newByteSink(ctx, r.pkr.w),
		source: newByteSource(ctx, r.bpool),

		Method:  method,
		RawArgs: argData,
	}
	req.sink.pkt.Flag = req.sink.pkt.Flag.Set(encFlag).Set(codec.FlagStream)
	req.Stream = req.sink.AsStream()

	if err := r.start(ctx, req); err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	return req.sink, nil
}

// Duplex does a duplex call on the remote.
func (r *rpc) Duplex(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSource, *ByteSink, error) {
	_, ok := r.manifest.Handled(method)
	if !ok {
		return nil, nil, fmt.Errorf("muxrpc(%s): method is not on their manifest", method)
	}

	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, nil, err
	}

	encFlag, err := re.asCodecFlag()
	if err != nil {
		return nil, nil, err
	}

	bSrc := newByteSource(ctx, r.bpool)
	bSink := newByteSink(ctx, r.pkr.w)
	bSink.pkt.Flag = bSink.pkt.Flag.Set(encFlag).Set(codec.FlagStream)

	req := &Request{
		Type: "duplex",

		source: bSrc,
		sink:   bSink,

		Method:  method,
		RawArgs: argData,
	}

	req.Stream = &streamDuplex{bSrc.AsStream(), bSink.AsStream()}

	if err := r.start(ctx, req); err != nil {
		return nil, nil, fmt.Errorf("error sending request: %w", err)
	}

	return bSrc, bSink, nil
}

// start starts a new call by allocating a request id and sending the first packet
func (r *rpc) start(ctx context.Context, req *Request) error {
	if req.abort == nil {
		req.abort = func() {} // noop
	}

	if req.RawArgs == nil {
		req.RawArgs = []byte("[]")
	}

	var (
		first codec.Packet
		err   error

		dbg = log.With(level.Debug(r.logger),
			"call", req.Type,
			"method", req.Method.String())
	)

	func() {
		r.rLock.Lock()
		defer r.rLock.Unlock()

		first.Flag = first.Flag.Set(codec.FlagJSON)
		first.Flag = first.Flag.Set(req.Type.Flags())
		first.Body, err = json.Marshal(req)

		r.highest++
		first.Req = r.highest
		r.reqs[first.Req] = req

		req.id = first.Req
		req.sink.pkt.Req = first.Req
	}()
	if err != nil {
		dbg.Log("event", "request create failed", "err", err)
		return err
	}

	dbg = log.With(dbg, "reqID", req.id)

	err = r.pkr.w.WritePacket(&first)
	if err != nil {
		return err
	}

	dbg.Log("event", "request sent", "flag", first.Flag.String())

	return nil
}

func (r *rpc) retreiveManifest() {

	var req = Request{
		Type: "sync",

		sink:   newByteSink(r.serveCtx, r.pkr.w),
		source: newByteSource(r.serveCtx, r.bpool),

		Method:  Method{"manifest"},
		RawArgs: json.RawMessage(`[]`),

		abort: func() {},
	}

	var (
		first codec.Packet
		err   error

		dbg = log.With(level.Warn(r.logger),
			"call", "manifest",
		)
	)

	func() {
		r.rLock.Lock()
		defer r.rLock.Unlock()

		first.Flag = first.Flag.Set(codec.FlagJSON)
		first.Body = []byte(`{"name":"manifest","args":[],"type":"async"}`)

		r.highest++
		first.Req = r.highest
		r.reqs[first.Req] = &req

		req.id = first.Req
		req.sink.pkt.Req = first.Req
	}()
	if err != nil {
		dbg.Log("event", "request create failed", "err", err)
		r.noManifest = true
		return
	}

	dbg = log.With(dbg, "reqID", req.id)

	err = r.pkr.w.WritePacket(&first)
	if err != nil {
		dbg.Log("event", "manifest request failed to send", "err", err)
		r.noManifest = true
		return
	}

	dbg.Log("event", "request sent", "flag", first.Flag.String())

	if !req.source.Next(r.serveCtx) {
		dbg.Log("event", "manifest request failed to read", "err", req.source.Err())
		r.noManifest = true
		return
	}

	manifestBody, err := req.source.Bytes()
	if err != nil {
		dbg.Log("event", "manifest request has no body?", "err", err)
		r.noManifest = true
		return
	}

	err = json.Unmarshal(manifestBody, &r.manifest)
	if err != nil {
		dbg.Log("event", "manifest request is invalid json", "err", err)
		r.noManifest = true
		return
	}

}

type manifestMap map[string]string

type manifestStruct struct {
	methods manifestMap
}

func (ms manifestStruct) Handled(m Method) (string, bool) {
	callType, yes := ms.methods[m.String()]
	return callType, yes
}

func (ms *manifestStruct) UnmarshalJSON(bin []byte) error {
	var dullMap map[string]interface{}

	err := json.Unmarshal(bin, &dullMap)
	if err != nil {
		return err
	}
	fmt.Printf("\n========\nFrom JSON:\n========\n%v\n", dullMap)

	methods := make(manifestMap)

	if err := recurseMap(methods, dullMap, nil); err != nil {
		return err
	}

	fmt.Printf("\n========\nUnpacked:\n========\n%#v\n", methods)

	ms.methods = methods
	return nil
}

/* recurseMap iterates over and decends into a muxrpc manifest and creates a flat structure ala

"plugin.method1": "async",
"plugin.method2": "source",
"plugin.method3": "sink",
...

*/
func recurseMap(methods manifestMap, jsonMap map[string]interface{}, prefix Method) error {
	for k, iv := range jsonMap {
		switch tv := iv.(type) {
		case string: // string means that's a method
			m := append(prefix, k).String()
			// fmt.Printf("method:%s - %s\n", m, tv)
			methods[m] = tv

		case map[string]interface{}: // map means it's a plugin group
			err := recurseMap(methods, tv, append(prefix, k))
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unhandled type in map: %T", iv)
		}
	}

	return nil
}
