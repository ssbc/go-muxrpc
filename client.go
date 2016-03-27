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
	var pkt codec.Packet
	pkt.Req = seq
	pkt.Type = call.Type // TODO: non JSON request

	var req Request
	req.Name = []string{call.Method} // TODO: nested methods
	req.Args = []interface{}{}
	if call.Args != nil {
		req.Args = []interface{}{call.Args} // TODO: multiple arguments
	}
	if call.stream {
		req.Type = "source"
		pkt.Stream = true
	}

	var err error
	if pkt.Body, err = json.Marshal(req); err != nil {
		client.mutex.Lock()
		delete(client.pending, seq)
		client.mutex.Unlock()
		call.Error = errgo.Notef(err, "muxrpc/call: body json.Marshal() failed")
		call.done()
		return
	}
	pkt.Len = uint32(len(pkt.Body))

	if err := client.w.WritePacket(&pkt); err != nil {
		client.mutex.Lock()
		delete(client.pending, seq)
		client.mutex.Unlock()
		call.Error = errgo.Notef(err, "muxrpc/call: WritePacket() failed")
		call.done()
	}
}

func (client *Client) read() {
	var err error
	var pkt *codec.Packet
	for err == nil {
		pkt, err = client.r.ReadPacket()
		if err != nil {
			xlog.Error("ReadPacket error:", err)
			break
		}
		seq := -pkt.Req
		client.mutex.Lock()
		call := client.pending[seq]
		if !call.stream || (pkt.Stream && pkt.EndErr) {
			delete(client.pending, seq)
		}
		client.mutex.Unlock()

		switch {
		case call == nil: // no call with this ID
			// TODO: check above will panic anyway in this case...
			xlog.Warn("could not handle incomming pkt: %s", pkt)
		case pkt.EndErr:
			// TODO: difference between End and Error?
			if pkt.Stream {
			} else {
				// We've got an error response. Give this to the request;
				// any subsequent requests will get the ReadResponseBody
				// error if there is one.
				call.Error = ServerError(string(pkt.Body))
			}
			call.done()
		default:
			switch pkt.Type {
			case codec.JSON:
				var um interface{}
				if err := json.Unmarshal(pkt.Body, &um); err != nil {
					call.Error = errgo.Notef(err, "muxrpc: unmarshall error")
					call.done()
					break
				}
				if call.stream {
					call.data <- um
				} else {
					call.Reply = um
					call.done()
				}

			case codec.String:
				if call.stream {
					// TODO
					call.Error = errgo.New("muxrpc: unhandeld encoding (string stream)")
					call.done()
					break
				} else {
					sptr, ok := call.Reply.(*string)
					if !ok {
						call.Error = errgo.New("muxrpc: illegal reply argument. wanted (*string)")
						call.done()
						break
					}
					*sptr = string(pkt.Body)
					call.done()
				}

			default:
				call.Error = errgo.Newf("muxrpc: unhandled pkt.Type %s", pkt)
				call.done()
				break
			}
		}
	}
	xlog.Error("input() loop broken. Err:", err)
	// Terminate pending calls.
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
	if err != io.EOF && !closing {
		xlog.Warn("rpc: client protocol error:", err)
	}

}

// Request is the Body value for rpc calls
// TODO: might fit into Call cleaner
type Request struct {
	Name []string      `json:"name"`
	Args []interface{} `json:"args"`
	Type string        `json:"type,omitempty"`
}

type Call struct {
	Method string // The name of the service and method to call.
	Type   codec.PacketType
	Args   interface{} // The argument to the function (*struct).
	Reply  interface{} // The reply from the function (*struct).
	Error  error       // After completion, the error status.
	Done   chan *Call  // Strobes when call is complete.

	data   chan interface{}
	stream bool
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		if call.stream {
			close(call.data)
		}
	default:
		xlog.Debug("muxrpc: discarding Call reply due to insufficient Done chan capacity")
	}
}

// Go invokes the function asynchronously.  It returns the Call structure representing
// the invocation.  The done channel will signal when the call is complete by returning
// the same Call object.  If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
func (client *Client) Go(call *Call, done chan *Call) *Call {
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
func (client *Client) Call(method string, args interface{}, reply interface{}) error {
	var c Call
	c.Method = method
	c.Args = args
	c.Reply = reply
	c.Type = codec.JSON // TODO: find other example
	call := <-client.Go(&c, make(chan *Call, 1)).Done
	return call.Error
}

func (client *Client) SyncSource(method string, args interface{}, reply interface{}) error {
	var c Call
	c.Method = method
	c.Args = args
	c.Reply = reply // TODO: useless
	c.Type = codec.JSON
	data := make(chan interface{})
	c.data = data
	c.stream = true
	client.Go(&c, make(chan *Call, 1))
	arr, ok := reply.(*[]int)
	if !ok {
		return errgo.Newf("reply not usable: %T", reply)
	}
	for d := range c.data {
		i, ok := d.(float64)
		if !ok {
			return errgo.Newf("data not usable: %T", d)
		}
		*arr = append(*arr, int(i))
	}
	call := <-c.Done
	return call.Error
}

func (c *Client) Close() error {
	return c.w.Close() // also closes the underlying con
}
