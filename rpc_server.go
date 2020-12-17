// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/karrick/bufpool"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

var (
	_ Endpoint = (*rpc)(nil)
	_ Server   = (*rpc)(nil)
)

// rpc implements an Endpoint, but also implements Server
type rpc struct {
	logger log.Logger

	remote net.Addr

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

	cancel context.CancelFunc
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
		return nil, false, errors.Wrap(err, "error parsing request")
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
		return nil, errors.New("expected negative request id")
	}

	// buf := r.bpool.Get()
	reqBody := make([]byte, pkt.Len)
	rd := r.pkr.r.NextBodyReader(pkt.Len)

	_, err := io.ReadFull(rd, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "error copying request body")
	}

	// TODO: move be before reading the body - just debugging
	if !pkt.Flag.Get(codec.FlagJSON) {
		return nil, fmt.Errorf("expected JSON flag (%d)", len(reqBody))
	}

	var req Request
	err = json.Unmarshal(reqBody, &req)
	// err := json.NewDecoder(rd).Decode(&req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nSPEW\n%s", spew.Sdump(reqBody))
		return nil, errors.Wrap(err, "error decoding packet")
	}

	// horray!
	req.id = pkt.Req

	todoCtx := context.TODO() // merge in with v2 changes to get serve() context

	// the capabailitys are nice guard rails but in the end
	// we still need to be able to error on a source
	// ie write to something which is supposed to be read-only.

	req.sink = newByteSink(todoCtx, r.pkr.w)
	req.sink.pkt.Req = req.id
	req.sink.pkt.Flag = pkt.Flag

	req.source = newByteSource(todoCtx, r.bpool)
	req.source.hdrFlag = pkt.Flag

	if pkt.Flag.Get(codec.FlagStream) {
		switch req.Type {
		case "duplex":
			return nil, fmt.Errorf("TODO: duplex legacy stream")
		case "source":
			req.Stream = req.sink.AsStream()
		case "sink":
			req.Stream = req.source.AsStream()
		default:
			return nil, errors.Errorf("unhandled request type: %q", req.Type)
		}
	} else {
		if req.Type == "" {
			req.Type = "async"
		}
		if req.Type != "async" {
			return nil, errors.Errorf("unhandled request type: %q", req.Type)
		}
		req.Stream = req.sink.AsStream()
	}

	level.Debug(r.logger).Log("event", "got request", "reqID", req.id, "method", req.Method, "type", req.Type)

	return &req, nil
}

// Server can handle packets to and from a remote party
type Server interface {
	Remote() net.Addr
	Serve(context.Context) error
}

// Serve drains the incoming packets and handles the RPC session
func (r *rpc) Serve(ctx context.Context) (err error) {
	level.Debug(r.logger).Log("event", "serving")
	defer func() {
		if luigi.IsEOS(err) || isAlreadyClosed(err) {
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
			err = r.pkr.NextHeader(ctx, &hdr)
			if luigi.IsEOS(err) || isAlreadyClosed(err) {
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
				err = errors.Wrap(err, "serve failed to read from packer")
				return true
			}

			return false
		}()
		if doRet {
			r.Terminate()
			return
		}

		// error handling and cleanup
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
					return errors.Wrapf(err, "muxrpc: failed to get error body for closing of req: %d (len:%d)", hdr.Req, hdr.Len)
				}

				body := buf.Bytes()

				if !isTrue(body) {
					streamErr, err = parseError(body)
					if err != nil {
						return errors.Wrap(err, "error parsing error packet")
					}
				}
				go func() {
					r.closeStream(req, streamErr)
					r.bpool.Put(buf)
				}()
			} else {
				level.Warn(r.logger).Log("event", "unhandled packet", "reqID", hdr.Req)
			}
			continue
		}

		var isNew bool
		req, isNew, err = r.fetchRequest(ctx, &hdr)
		if err != nil {
			err = errors.Wrap(err, "muxrpc: error unpacking request")
			return
		}
		if isNew {
			continue
		}

		err = req.source.consume(hdr.Len, r.pkr.r.NextBodyReader(hdr.Len))
		if err != nil {
			err = errors.Wrap(err, "muxrpc: error pouring data to handler")
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
		return nil, errors.Wrap(err, "error unmarshaling error packet")
	}

	// There are also TypeErrors and numerous other things we might get from this..
	// if e.Name != "Error" {
	// 	return nil, errors.Errorf(`name is not "Error" but %q`, e.Name)
	// }

	return &e, nil
}
