package muxrpc

import (
	"context"
	"net"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

// TestPacker is a crappy golden path test
func TestPacker(t *testing.T) {
	c1, c2 := net.Pipe()

	pkr1 := NewPacker(c1)
	pkr2 := NewPacker(c2)

	ctx := context.Background()

	pkt := codec.Packet{
		Req:  123,
		Flag: codec.FlagString | codec.FlagStream,
		Body: []byte("wat"),
	}

	errc := make(chan error)
	go func() {
		err := pkr1.Pour(ctx, &pkt)
		if err != nil {
			errc <- errors.Wrap(err, "failed to send test packet (wat)")
		}
		close(errc)
	}()

	v, err := pkr2.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pkt_, ok := v.(*codec.Packet)
	if !ok {
		t.Fatalf("expected a %T, got %v(%T)", pkt_, v, v)
	}

	pkt.Req = -123

	if !reflect.DeepEqual(pkt_, &pkt) {
		t.Log("Req matches:", reflect.DeepEqual(pkt.Req, pkt_.Req))
		t.Log("Flag matches:", reflect.DeepEqual(pkt.Flag, pkt_.Flag))
		t.Log("Body matches:", reflect.DeepEqual(pkt.Body, pkt_.Body))
		t.Log("Packet matches:", reflect.DeepEqual(&pkt, pkt_))
		t.Logf("expected packet %#v", &pkt)
		t.Logf("got %#v", pkt_)
		t.Fatal("packet mismatch")
	}

	err = pkr1.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = pkr2.Next(ctx)
	if !luigi.IsEOS(err) {
		t.Fatal("expected EOS, got:", err)
	}

	err = pkr1.Pour(ctx, pkt_)
	if err == nil {
		t.Fatal("expected write-to-close-conn error, got nil")
	}

	sendErr, ok := <-errc
	if ok {
		t.Fatal(sendErr)
	}

	t.Log("this error should be about pouring to a closed sink:", err)
}
