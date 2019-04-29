package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"

	"go.cryptoscope.co/muxrpc/codec"
	"go.cryptoscope.co/muxrpc/debug"
)

func initLogging(t *testing.T, pref string) (l log.Logger, w io.Writer) {
	w = logtest.Logger(pref, t)
	if testing.Verbose() {
		w = io.MultiWriter(w, os.Stderr)
	}
	l = log.NewLogfmtLogger(w)
	return
}

// for some reason you can't use t.Fatal // t.Error in goroutines... :-/
func serve(ctx context.Context, r Server, errc chan<- error, done ...chan<- struct{}) {
	err := r.Serve(ctx)
	if err != nil {
		errc <- errors.Wrap(err, "Serve failed")
	}
	if len(done) > 0 { // might want to use a waitGroup here instead?
		close(done[0])
	}
}

func mkCheck(errc chan<- error) func(err error) {
	return func(err error) {
		if err != nil {
			errc <- err
		}
	}
}

func rewrap(l log.Logger, p Packer) Packer {
	ip, ok := p.(*packer)
	if !ok {
		l.Log(
			"rewrap", "warn: wrong internal packer type",
			"t", fmt.Sprintf("%T", p))
		return p
	}

	rwc, ok := ip.c.(io.ReadWriteCloser)
	if !ok {
		panic(fmt.Sprintf("expected RWC: %T", ip.c))
	}

	return NewPacker(debug.Wrap(l, rwc))
}

func BuildTestAsync(pkr1, pkr2 Packer) func(*testing.T) {
	return func(t *testing.T) {
		r := require.New(t)

		conn2 := make(chan struct{})
		conn1 := make(chan struct{})
		serve1 := make(chan struct{})
		serve2 := make(chan struct{})

		errc := make(chan error)

		var fh1 FakeHandler
		fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h1 connected")
			close(conn1) // I think this _should_ terminate e?
		})

		var fh2 FakeHandler
		fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
			t.Logf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				err := req.Return(ctx, "you are a test")
				if err != nil {
					errc <- errors.Wrap(err, "return errored")
				}
			}
		})

		fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h2 connected")
			close(conn2)
		})

		if testing.Verbose() {
			muxlog1, _ := initLogging(t, "p1")
			muxlog2, _ := initLogging(t, "p2")
			pkr1 = rewrap(muxlog1, pkr1)
			pkr2 = rewrap(muxlog2, pkr2)
		}

		rpc1 := Handle(pkr1, &fh1)
		rpc2 := Handle(pkr2, &fh2)

		ctx := context.Background()

		go serve(ctx, rpc1.(Server), errc, serve1)
		go serve(ctx, rpc2.(Server), errc, serve2)

		v, err := rpc1.Async(ctx, "string", Method{"whoami"})
		r.NoError(err)
		r.Equal("you are a test", v)

		time.Sleep(time.Millisecond)

		err = rpc1.Terminate()
		t.Log("waiting for closes")

		go func() {
			for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
				select {
				case <-conn1:
					t.Log("conn1 closed")
					conn1 = nil
				case <-conn2:
					t.Log("conn2 closed")
					conn2 = nil
				case <-serve1:
					t.Log("serve1 closed")
					serve1 = nil
				case <-serve2:
					t.Log("serve2 closed")
					serve2 = nil
				}
			}
			t.Log("done")
			close(errc)
		}()

		for err := range errc {
			if err != nil {
				t.Fatalf("from error chan:\n%+v", err)
			}
		}

		r.Equal(0, fh1.HandleCallCallCount(), "peer h2 did call unexpectedly")
	}
}

func TestAsync(t *testing.T) {
	type duplex struct {
		luigi.Source
		luigi.Sink
	}

	negReqMapFunc := func(ctx context.Context, v interface{}) (interface{}, error) {
		pkt := v.(*codec.Packet)
		pkt.Req = -pkt.Req
		return pkt, nil
	}
	type makerFunc func() (string, Packer, Packer)
	pkrgens := []makerFunc{
		func() (string, Packer, Packer) {
			c1, c2 := net.Pipe()
			p1, p2 := NewPacker(c1), NewPacker(c2)
			return "NetPipe", p1, p2
		},
		func() (string, Packer, Packer) {
			rxSrc, rxSink := luigi.NewPipe(luigi.WithBuffer(5))
			txSrc, txSink := luigi.NewPipe(luigi.WithBuffer(5))

			rxSrc = mfr.SourceMap(rxSrc, negReqMapFunc)
			txSrc = mfr.SourceMap(txSrc, negReqMapFunc)

			return "LuigiPipes (buffered)", duplex{rxSrc, txSink}, duplex{txSrc, rxSink}
		},
		func() (string, Packer, Packer) {
			rxSrc, rxSink := luigi.NewPipe()
			txSrc, txSink := luigi.NewPipe()

			rxSrc = mfr.SourceMap(rxSrc, negReqMapFunc)
			txSrc = mfr.SourceMap(txSrc, negReqMapFunc)

			return "LuigiPipes (unbuffered)", duplex{rxSrc, txSink}, duplex{txSrc, rxSink}
		},
	}

	for _, pkrgen := range pkrgens {
		name, pkr1, pkr2 := pkrgen()
		t.Run(name, BuildTestAsync(pkr1, pkr2))
	}
}

func TestSource(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Log("h1 called")
		errc <- errors.Errorf("unexpected call to rpc1: %#v", req)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for _, v := range expRx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(errors.Wrap(err, "h2 pour errored"))
			}
			err := req.Stream.Close()
			ckFatal(errors.Wrap(err, "h2 end pour errored"))
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	src, err := rpc1.Source(ctx, "strings", Method{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, exp := range expRx {
		v, err := src.Next(ctx)
		if err != nil {
			t.Errorf("next failed: %+v", err)
		}

		if v != exp {
			t.Errorf("unexpected response message %q, expected %v", v, exp)
		}
	}

	val, err := src.Next(ctx)
	if !luigi.IsEOS(err) {
		t.Errorf("expected end of stream, got value %v and error %+v", val, err)
	}

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("from errc: %+v", err)
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer did not call 'connect'")
}

func TestSink(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})
	wait := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for i, exp := range expRx {
				t.Log("calling Next()", i)
				v, err := req.Stream.Next(ctx)
				ckFatal(err)
				t.Log("Next()", i, "returned", v)

				if v != exp {
					err = errors.Errorf("expected value %v, got %v", exp, v)
					ckFatal(err)
				}
			}

			close(wait)

			err := req.Stream.Close()
			ckFatal(err)

		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	sink, err := rpc1.Sink(ctx, Method{"whoami"})
	r.NoError(err)

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		r.NoError(err)
	}
	t.Log("waiting for wait...")
	<-wait
	t.Log("got wait!")
	err = sink.Close()
	r.NoError(err)

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}
}

func TestDuplex(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := net.Pipe()

	var wg sync.WaitGroup
	wg.Add(3)

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for _, exp := range expRx {
				v, err := req.Stream.Next(ctx)
				ckFatal(errors.Wrap(err, "pour errored"))

				if v != exp {
					err = errors.Errorf("expected value %v, got %v", exp, v)
					ckFatal(err)
				}
			}

			for _, v := range expTx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(err)
			}

			err := req.Stream.Close()
			ckFatal(err)
			wg.Done()
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		wg.Done()
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	src, sink, err := rpc1.Duplex(ctx, "str", Method{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		if err != nil {
			t.Error(err)
		}
	}

	for _, exp := range expTx {
		v, err := src.Next(ctx)
		if err != nil {
			t.Error(err)
		}

		if v != exp {
			t.Errorf("expected %v, got %v", exp, v)
		}
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")
	r.Equal(1, fh2.HandleCallCallCount(), "peer h2 did call unexpectedly")
}

// TODO: soome weirdness - see TestJSSyncString for error handling
func XTestErrorAsync(t *testing.T) {
	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error, 10)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			err := req.Stream.CloseWithError(errors.New("omg an error!"))
			ckFatal(err)
		} else {
			err := req.Stream.CloseWithError(errors.New("unexpected !"))
			ckFatal(err)
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	pktLog, _ := initLogging(t, "packer")
	pkr1 := NewPacker(debug.Wrap(pktLog, c1))

	rpc1 := Handle(pkr1, &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	go func() {
		v, err := rpc1.Async(ctx, "string", Method{"whoami"})
		if err == nil {
			ckFatal(fmt.Errorf("expected an error"))
		} else if errors.Cause(err).Error() != "omg an error!" {
			ckFatal(fmt.Errorf("expected error %q, got %q", "omg an error!", errors.Cause(err)))
		}

		e, ok := errors.Cause(err).(*CallError)
		if !ok {
			t.Fatalf("not a callerror!")
		}

		if e.Message != "omg an error!" {
			ckFatal(fmt.Errorf("unexpected error message"))
		}

		if e.Name != "Error" {
			ckFatal(fmt.Errorf("expected field name to have %q, got %q", "Error", e.Name))
		}

		if v != nil {
			ckFatal(fmt.Errorf("unexpected response message %q, expected nil", v))
		}
	}()

	t.Log("waiting for closes")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatal(err)
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}
	t.Log("done")
}

type testStruct struct {
	A   int
	N   int
	Str string
}
type hDuplex struct {
	logger         log.Logger
	txvals, rxvals []interface{}
	failed         chan error
}

func (h *hDuplex) HandleConnect(ctx context.Context, e Endpoint) {
	h.logger.Log("connect:", e.Remote())
}

func (h *hDuplex) HandleCall(ctx context.Context, req *Request, edp Endpoint) {
	defer close(h.failed)

	if req.Method.String() != "magic" {
		edp.Terminate()
		return
	}

	if req.Type != "duplex" {
		edp.Terminate()
		return
	}
	h.logger.Log("correct", "signature")

	go func() {
		for i, tx := range h.txvals {
			err := req.Stream.Pour(ctx, tx)
			if err != nil {
				h.failed <- errors.Wrapf(err, "failed to tx val %d", i)
				return
			}
			h.logger.Log("event", "sent", "err", err, "i", i, "tx", fmt.Sprintf("%+v", tx))
			time.Sleep(250 * time.Millisecond)
		}
		// req.Stream.Close()
	}()

	j := 0
	for {
		v, err := req.Stream.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			h.failed <- errors.Wrap(err, "drined input stream")
			return
		}
		switch rv := v.(type) {
		case float64:
			h.rxvals = append(h.rxvals, rv)
		case string:
			h.rxvals = append(h.rxvals, rv)
		case map[string]interface{}:
			var v testStruct
			v.A = int(rv["A"].(float64))
			v.N = int(rv["N"].(float64))
			v.Str = rv["Str"].(string)
			h.rxvals = append(h.rxvals, v)
		default:
			h.failed <- errors.Errorf("got unhandled duplex msg type: %T", v)
			return
		}
		j++
	}
	// req.Stream.Close()
}
func TestDuplexHandlerStr(t *testing.T) {
	dbg, w := logtest.KitLogger("dph", t)
	if testing.Verbose() {

		dbg = log.NewLogfmtLogger(io.MultiWriter(w, os.Stderr))

	}
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := net.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)

	errc := make(chan error)
	// ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var h2 hDuplex
	h2.logger = dbg
	h2.failed = make(chan error)

	h2.txvals = make([]interface{}, len(expTx))
	for i, v := range expTx {
		h2.txvals[i] = v
	}
	go func() {
		err := <-h2.failed
		if err != nil {
			errc <- err
		}
		wg.Done()
	}()

	rpc1 := Handle(NewPacker(debug.Wrap(dbg, c1)), &fh1)
	rpc2 := Handle(NewPacker(c2), &h2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	src, sink, err := rpc1.Duplex(ctx, "str", Method{"magic"})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		if err != nil {
			t.Error(err)
		}
	}

	for _, exp := range expTx {
		v, err := src.Next(ctx)
		if err != nil {
			t.Error(err)
		}

		if v != exp {
			t.Errorf("expected %v, got %v", exp, v)
		}
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	go func() {
		wg.Wait()
		close(errc)
	}()

	r.Len(h2.rxvals, len(expRx))
	for i, v := range h2.rxvals {
		r.EqualValues(expRx[i], v, "value %d", i)
	}

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")
}

func TestDuplexHandlerJSON(t *testing.T) {

	// dbg, _ := logtest.KitLogger("dph", t)
	dbg := log.NewLogfmtLogger(os.Stderr)
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := net.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)

	errc := make(chan error, 2)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var h2 hDuplex
	h2.logger = dbg
	h2.failed = make(chan error)

	h2.txvals = make([]interface{}, len(expTx))
	for i, v := range expTx {
		h2.txvals[i] = testStruct{
			A:   i,
			N:   len(expTx),
			Str: v,
		}
	}

	go func() {
		err := <-h2.failed
		if err != nil {
			dbg.Log("warning", "error from handler", "err", err)
			errc <- err
		}
		wg.Done()
	}()

	rpc1 := Handle(NewPacker(debug.Wrap(dbg, c1)), &fh1)
	rpc2 := Handle(NewPacker(c2), &h2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	var v interface{}
	src, sink, err := rpc1.Duplex(ctx, v, Method{"magic"})
	r.NoError(err)

	for i, v := range expRx {
		err := sink.Pour(ctx, testStruct{
			A:   i,
			N:   len(expRx),
			Str: v,
		})
		r.NoError(err)
	}

	for _, exp := range expTx {
		v, err := src.Next(ctx)
		r.NoError(err)
		mv, ok := v.(map[string]interface{})
		r.True(ok)
		r.EqualValues(exp, mv["Str"])
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		r.NoError(err)
	}

	r.Len(h2.rxvals, len(expRx))
	for i, v := range h2.rxvals {
		ts := v.(testStruct)
		r.EqualValues(i, ts.A, "value %d", i)
		r.EqualValues(expRx[i], ts.Str, "value %d", i)
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")

}
