// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/karrick/bufpool"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// HandleOption are used to configure rpc handler instances
type HandleOption func(*rpc)

// WithContext sets the context for the whole lifetime of the rpc session
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

// WithIsServer sets wether the Handle should be in the server (true) or client (false) role
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

	// apply options
	for _, o := range opts {
		o(r)
	}

	// defaults
	if r.logger == nil {
		logger := log.NewLogfmtLogger(os.Stderr)
		logger = level.NewFilter(logger, level.AllowInfo()) // only log info and above
		r.logger = log.With(logger, "ts", log.DefaultTimestampUTC, "unit", "muxrpc")
	}

	if r.remote == nil {
		if ra, ok := pkr.c.(interface{ RemoteAddr() net.Addr }); ok {
			r.remote = ra.RemoteAddr()
		}
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
			return nil, fmt.Errorf("error marshaling request arguments: %w", err)
		}
	}
	return argData, nil
}

var (
	_ Endpoint = (*rpc)(nil)
	_ Server   = (*rpc)(nil)
)

// rpc implements an Endpoint, but also implements Server
type rpc struct {
	logger log.Logger

	remote net.Addr

	isServer bool // is this rpc endpoint in the server role?

	// pkr (un)marshales codec.Packets
	pkr *Packer

	bpool bufpool.FreeList

	// reqs is the map we keep, tracking all requests
	reqs  map[int32]*Request
	rLock sync.RWMutex

	// highest is the highest request id we already allocated
	highest int32

	root Handler

	// terminated indicates that the rpc session is being terminated
	terminated bool
	tLock      sync.Mutex

	serveCtx context.Context
	cancel   context.CancelFunc
}

// fetchRequest returns the request from the reqs map or, if it's not there yet, builds a new one.
func (r *rpc) fetchRequest(ctx context.Context, hdr *codec.Header) (*Request, bool, error) {
	var err error

	r.rLock.RLock()

	// get request from map, otherwise make new one
	req, exists := r.reqs[hdr.Req]
	if exists {
		r.rLock.RUnlock()
		return req, false, nil
	}
	r.rLock.RUnlock()
	r.rLock.Lock()
	defer r.rLock.Unlock()

	req, err = r.parseNewRequest(hdr)
	if err != nil {
		return nil, false, fmt.Errorf("error parsing request: %w", err)
	}
	ctx, req.abort = context.WithCancel(ctx)

	r.reqs[hdr.Req] = req
	// TODO:
	// buffer new requests to not mindlessly spawn goroutines
	// and prioritize exisitng requests to unblock the connection time
	// maybe use two maps
	go func() {
		r.root.HandleCall(ctx, req, r)
		level.Debug(r.logger).Log("call", "returned", "method", req.Method, "reqID", req.id)
	}()
	return req, true, nil
}

// parseNewRequest parses the first packet of a stream and parses the contained request
func (r *rpc) parseNewRequest(pkt *codec.Header) (*Request, error) {
	if pkt.Req >= 0 {
		// request numbers should have been inverted by now
		return nil, errors.New("muxrpc: expected negative request id")
	}

	if !pkt.Flag.Get(codec.FlagJSON) {
		return nil, errors.New("muxrpc: expected JSON flag for call manifest")
	}

	buf := r.bpool.Get()
	buf.Reset()

	// make sure the whole body fits
	if n := buf.Cap() - int(pkt.Len); n < 0 {
		buf.Grow(-n)
	}

	rd := r.pkr.r.NextBodyReader(pkt.Len)
	_, err := buf.ReadFrom(rd)
	if err != nil {
		return nil, fmt.Errorf("error copying request body: %w", err)
	}

	var req Request
	err = json.NewDecoder(buf).Decode(&req)
	if err != nil {
		return nil, fmt.Errorf("error decoding packet: %w", err)
	}

	r.bpool.Put(buf)

	req.id = pkt.Req

	// the capabailitys are nice guard rails but in the end
	// we still need to be able to error on a source
	// ie write to something which is supposed to be read-only.

	req.sink = newByteSink(r.serveCtx, r.pkr.w)
	req.sink.pkt.Req = req.id

	req.source = newByteSource(r.serveCtx, r.bpool)

	if pkt.Flag.Get(codec.FlagStream) {
		req.sink.pkt.Flag = req.sink.pkt.Flag.Set(codec.FlagStream)
		switch req.Type {
		case "duplex":
			req.Stream = &streamDuplex{src: req.source.AsStream(), snk: req.sink.AsStream()}
		case "source":
			req.Stream = req.sink.AsStream()
		case "sink":
			req.Stream = req.source.AsStream()
		default:
			return nil, fmt.Errorf("unhandled request type: %q", req.Type)
		}
	} else {
		if req.Type == "" {
			req.Type = "async"
		}
		if req.Type != "async" {
			return nil, fmt.Errorf("unhandled request type: %q", req.Type)
		}
		req.Stream = req.sink.AsStream()
	}

	level.Debug(r.logger).Log("event", "got request", "reqID", req.id, "method", req.Method, "type", req.Type)

	return &req, nil
}

// Server can handle packets to and from a remote party
type Server interface {
	Remote() net.Addr
	Serve() error
}

// Serve drains the incoming packets and handles the RPC session
func (r *rpc) Serve() (err error) {
	level.Debug(r.logger).Log("event", "serving")
	defer func() {
		if isAlreadyClosed(err) {
			err = nil
		}
		cerr := r.pkr.Close()
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			level.Info(r.logger).Log("event", "closed", "handleErr", err, "closeErr", cerr)
		}
	}()

	for {
		var hdr codec.Header

		// read next packet from connection
		doRet := func() bool {
			err = r.pkr.NextHeader(r.serveCtx, &hdr)
			if isAlreadyClosed(err) {
				err = nil
				return true
			}

			r.tLock.Lock()
			defer r.tLock.Unlock()

			if err != nil {
				if r.terminated {
					err = nil
					return true
				}
				err = fmt.Errorf("muxrpc: serve failed to read from packer: %w", err)
				return true
			}

			return false
		}()
		if doRet {
			r.Terminate()
			return
		}

		// error/endstream handling and cleanup
		var req *Request
		if hdr.Flag.Get(codec.FlagEndErr) {
			getReq := func(req int32) (*Request, bool) {
				r.rLock.RLock()
				defer r.rLock.RUnlock()

				r, ok := r.reqs[req]
				return r, ok
			}

			var ok bool
			if req, ok = getReq(hdr.Req); ok {
				var streamErr error
				req.abort()

				buf := r.bpool.Get()

				err = r.pkr.r.ReadBodyInto(buf, hdr.Len)
				if err != nil {
					return fmt.Errorf("muxrpc: failed to get error body for closing of req: %d (len:%d): %w", hdr.Req, hdr.Len, err)
				}

				body := buf.Bytes()
				r.bpool.Put(buf)

				if !isTrue(body) {
					streamErr, err = parseError(body)
					if err != nil {
						return fmt.Errorf("error parsing error packet: %w", err)
					}
				}
				go func() {
					r.closeStream(req, streamErr)
				}()
			} else {
				level.Warn(r.logger).Log("event", "unhandled packet", "reqID", hdr.Req)
			}
			continue
		}

		// data muxing

		// pick the requests or create a new one
		var isNew bool
		req, isNew, err = r.fetchRequest(r.serveCtx, &hdr)
		if err != nil {
			err = fmt.Errorf("muxrpc: error unpacking request: %w", err)
			return
		}

		if isNew { // the first packet is just the request data, nothing else to do
			continue
		}

		err = req.source.consume(hdr.Len, hdr.Flag, r.pkr.r.NextBodyReader(hdr.Len))
		if err != nil {
			err = fmt.Errorf("muxrpc: error pouring data to handler: %w", err)
			return
		}
	}
}

func isTrue(data []byte) bool {
	return len(data) == 4 &&
		data[0] == 't' &&
		data[1] == 'r' &&
		data[2] == 'u' &&
		data[3] == 'e'
}

func (r *rpc) closeStream(req *Request, streamErr error) {
	err := req.CloseWithError(streamErr)
	if err != nil {
		level.Warn(r.logger).Log("event", "close stream failed", "reqID", req.id, "method", req.Method.String(), "err", err)
	}

	r.rLock.Lock()
	defer r.rLock.Unlock()
	delete(r.reqs, req.id)
	return
}

// ErrSessionTerminated is returned once Terminate() was called  or the connection dies
var ErrSessionTerminated = errors.New("muxrpc: session terminated")

// Terminate ends the RPC session
func (r *rpc) Terminate() error {
	r.cancel()
	r.tLock.Lock()
	defer r.tLock.Unlock()
	r.terminated = true
	r.rLock.Lock()
	defer r.rLock.Unlock()
	if n := len(r.reqs); n > 0 { // close active requests
		for _, req := range r.reqs {
			req.CloseWithError(ErrSessionTerminated)
		}
	}
	return r.pkr.Close()
}

func (r *rpc) Remote() net.Addr {
	return r.remote
}

// CallError is returned when a call fails
type CallError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

func (e *CallError) Error() string {
	return fmt.Sprintf("muxrpc CallError: %s - %s", e.Name, e.Message)
}

func parseError(data []byte) (*CallError, error) {
	var e CallError

	err := json.Unmarshal(data, &e)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling error packet: %w", err)
	}

	// There are also TypeErrors and numerous other things we might get from this..
	// if e.Name != "Error" {
	// 	return nil, fmt.Errorf(`name is not "Error" but %q`, e.Name)
	// }

	return &e, nil
}
