/* WIP/Ripoff off net/rpc

TODO: source streams
Endgame: codegen over muxrpc manifest
*/

package muxrpc

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"sync"

	"github.com/rs/xlog"
	"gopkg.in/errgo.v1"

	"github.com/cryptix/go-muxrpc/codec"
)

// ServerError represents an error that has been returned from
// the remote side of the RPC connection.
type ServerError string

func (e ServerError) Error() string {
	return string(e)
}

var ErrShutdown = errors.New("connection is shut down")

type Client struct {
	r *codec.Reader
	w *codec.Writer
	c io.ReadWriteCloser

	reqMutex sync.Mutex // protects following
	request  codec.Packet

	mutex    sync.Mutex // protects following
	seq      int32
	pending  map[int32]*Call
	closing  bool // user has called Close
	shutdown bool // server has told us to stop
}

func NewClient(rwc io.ReadWriteCloser) *Client {
	c := Client{
		r:       codec.NewReader(rwc),
		w:       codec.NewWriter(rwc),
		c:       rwc,
		seq:     1,
		pending: make(map[int32]*Call),
	}
	go c.read()
	return &c
}

func (client *Client) send(call *Call) {
	client.reqMutex.Lock()
	defer client.reqMutex.Unlock()

	// Register this call.
	client.mutex.Lock()
	if client.shutdown || client.closing {
		call.Error = ErrShutdown
		client.mutex.Unlock()
		call.done()
		return
	}
	seq := client.seq
	client.seq++
	client.pending[seq] = call
	client.mutex.Unlock()

	// Encode and send the request.
	client.request.Req = seq
	client.request.Type = codec.JSON

	var req Request
	req.Name = []string{call.Method}
	req.Args = []interface{}{call.Args}

	var err error
	if client.request.Body, err = json.Marshal(req); err != nil {
		client.mutex.Lock()
		call = client.pending[seq]
		delete(client.pending, seq)
		client.mutex.Unlock()
		if call != nil {
			call.Error = errgo.Notef(err, "muxrpc/call: body json.Marshal() failed")
			call.done()
		}
		return
	}

	client.request.Len = uint32(len(client.request.Body))

	if err := client.w.WritePacket(&client.request); err != nil {
		client.mutex.Lock()
		call = client.pending[seq]
		delete(client.pending, seq)
		client.mutex.Unlock()
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) read() {
	var err error
	var pkt *codec.Packet
	for err == nil {
		pkt, err = client.r.ReadPacket()
		if err != nil {
			break
		}
		seq := -pkt.Req
		client.mutex.Lock()
		call := client.pending[seq]
		if !call.stream {
			delete(client.pending, seq)
		}
		client.mutex.Unlock()

		switch {
		case call == nil:
			// We've got no pending call. That usually means that
			// WriteRequest partially failed, and call was already
			// removed; response is a server telling us about an
			// error reading request body. We should still attempt
			// to read error body, but there's no one to give it to.
			xlog.Warn("could not handle incomming pkt: %s", pkt)
		case pkt.EndErr:
			// We've got an error response. Give this to the request;
			// any subsequent requests will get the ReadResponseBody
			// error if there is one.
			call.Error = ServerError(string(pkt.Body))
			call.done()
		default:
			xlog.Infof("Normal response: %s", pkt)

			// demux reponse
			if call.stream {
				call.data <- pkt.Body
				xlog.Info("stream data send on channel")
			} else {
				switch pkt.Type {
				case codec.String:
					sptr, ok := call.Reply.(*string)
					if !ok {
						call.Error = errgo.New("muxrpc: illegal reply argument. wanted (*string)")
						call.done()
						return
					}
					*sptr = string(pkt.Body)

				default:
					call.Error = errgo.Newf("muxrpc: unhandled pkt.Type %s", pkt)
					call.done()
				}
			}
			call.done()
		}

	}
	// Terminate pending calls.
	client.reqMutex.Lock()
	client.mutex.Lock()
	client.shutdown = true
	closing := client.closing
	if err == io.EOF {
		if closing {
			err = ErrShutdown
		} else {
			err = io.ErrUnexpectedEOF
		}
	}
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
	client.mutex.Unlock()
	client.reqMutex.Unlock()
	if err != io.EOF && !closing {
		xlog.Warn("rpc: client protocol error:", err)
	}

}

type Request struct {
	Name []string      `json:"name"`
	Args []interface{} `json:"args"`
	Type string        `json:"type,omitempty"`
}

type Call struct {
	Method string      // The name of the service and method to call.
	Args   interface{} // The argument to the function (*struct).
	Reply  interface{} // The reply from the function (*struct).
	Error  error       // After completion, the error status.
	Done   chan *Call  // Strobes when call is complete.

	data   chan<- interface{}
	stream bool
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		// ok
	default:

		xlog.Debug("muxrpc: discarding Call reply due to insufficient Done chan capacity")
	}
}

// Go invokes the function asynchronously.  It returns the Call structure representing
// the invocation.  The done channel will signal when the call is complete by returning
// the same Call object.  If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
func (client *Client) Go(method string, args interface{}, reply interface{}, done chan *Call) *Call {
	call := new(Call)
	call.Method = method
	call.Args = args
	call.Reply = reply
	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		// If caller passes done != nil, it must arrange that
		// done has enough buffer for the number of simultaneous
		// RPCs that will be using that channel.  If the channel
		// is totally unbuffered, it's best not to run at all.
		if cap(done) == 0 {
			log.Panic("muxrpc: done channel is unbuffered")
		}
	}
	call.Done = done
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete, and returns its error status.
func (client *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}

func (c *Client) Close() error {
	return c.w.Close() // also closes the underlying con
}
