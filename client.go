/* WIP/Ripoff off net/rpc

TODO: source streams
Endgame: codegen over muxrpc manifest
*/

package muxrpc

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cryptix/go-muxrpc/codec"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
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

	log log.Logger // logging utility for unhandled calls etc
}

func NewClient(l log.Logger, rwc io.ReadWriteCloser) *Client {
	// TODO: pass in ctx
	c := Client{
		r:       codec.NewReader(rwc),
		w:       codec.NewWriter(rwc),
		c:       rwc,
		seq:     1,
		pending: make(map[int32]*Call),

		log: log.With(l, "unit", "muxrpc"),
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

	if methods := strings.Split(call.Method, "."); len(methods) > 1 {
		req.Name = methods
	} else {
		req.Name = []string{call.Method}
	}
	req.Args = []interface{}{}
	if call.Args != nil {
		req.Args = []interface{}{call.Args}
		/* TODO(cryptix): hacked this to test multiple arguments

		Maybe change the Call() signature to Call(method string, reply interface{}, args ...interface{})?
		*/
		if call.Method == "private.publish" {
			a := call.Args.(map[string]interface{})
			req.Args = make([]interface{}, 2)
			req.Args[0] = a["content"]
			req.Args[1] = a["rcps"]
		}
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
		call.Error = errors.Wrap(err, "muxrpc/call: body json.Marshal() failed")
		call.done()
		return
	}

	if err := client.w.WritePacket(&pkt); err != nil {
		client.mutex.Lock()
		delete(client.pending, seq)
		client.mutex.Unlock()
		call.Error = errors.Wrap(err, "muxrpc/call: WritePacket() failed")
		call.done()
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
		// TODO: this is... p2p! no srsly we might get called
		call, ok := client.pending[seq]
		if !ok {
			client.log.Log("warning", fmt.Sprintf("non-pending pkt: %s", pkt))
			client.mutex.Unlock()
			continue

		}
		if !call.stream || (pkt.Stream && pkt.EndErr) {
			delete(client.pending, seq)
		}
		client.mutex.Unlock()

		switch {
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
				// todo there sure is a nicer way to structure this
				if call.stream {
					var um interface{}
					if err := json.Unmarshal(pkt.Body, &um); err != nil {
						call.Error = errors.Wrap(err, "muxrpc: unmarshall error")
						call.done()
						break
					}
					call.data <- um
				} else {
					if err := json.Unmarshal(pkt.Body, call.Reply); err != nil {
						call.Error = errors.Wrap(err, "muxrpc: unmarshall error")
						call.done()
						break
					}
					call.done()
				}

			case codec.String:
				if call.stream {
					// TODO
					call.Error = errors.New("muxrpc: unhandeld encoding (string stream)")
					call.done()
					break
				} else {
					sptr, ok := call.Reply.(*string)
					if !ok {
						call.Error = errors.New("muxrpc: illegal reply argument. wanted (*string)")
						call.done()
						break
					}
					*sptr = string(pkt.Body)
					call.done()
				}

			default:
				call.Error = errors.Errorf("muxrpc: unhandled pkt.Type %s", pkt)
				call.done()
				break
			}
		}
	}
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
		client.log.Log("error", errors.Wrap(err, "rpc: client protocol error."))
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

	log log.Logger
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		if call.stream {
			close(call.data)
		}
	default:
		call.log.Log("debug/todo", "discarding Call reply due to insufficient Done chan capacity")
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
			msg := "done channel is unbuffered"
			client.log.Log("error", msg)
			panic(msg)
		}
	}
	call.Done = done
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete, and returns its error status.
func (client *Client) Call(method string, args interface{}, reply interface{}) error {
	var c Call
	c.log = log.With(client.log, "unit", "muxrpc/call", "method", method)
	c.Method = method
	c.Args = args
	c.Reply = reply
	c.Type = codec.JSON // TODO: find other example
	call := <-client.Go(&c, make(chan *Call, 1)).Done
	return call.Error
}

func (client *Client) SyncSource(method string, args interface{}, reply interface{}) error {
	var c Call
	c.log = log.With(client.log, "unit", "muxrpc/sync", "method", method)
	c.Method = method
	c.Args = args
	c.Reply = reply // TODO: useless
	c.Type = codec.JSON
	data := make(chan interface{})
	c.data = data
	c.stream = true
	client.Go(&c, make(chan *Call, 1))
	arr, ok := reply.(*[]map[string]interface{})
	if !ok {
		return errors.Errorf("reply not usable: %T", reply)
	}
	for d := range c.data {
		i, ok := d.(map[string]interface{})
		if !ok {
			return errors.Errorf("data not usable: %T", d)
		}
		*arr = append(*arr, i)
	}
	call := <-c.Done
	return call.Error
}

func (c *Client) Close() error {
	c.closing = true
	return c.w.Close() // also closes the underlying con
}
