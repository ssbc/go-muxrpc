package muxrpc

import (
	"bytes"
	"context"
	"net"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
	"go.cryptoscope.co/muxrpc/debug"
	"go.mindeco.de/logging/logtest"
)

// TestPacker is a crappy golden path test
func TestPacker(t *testing.T) {
	c1, c2 := net.Pipe()

	var pkr1 *Packer
	if testing.Verbose() {
		l, _ := logtest.KitLogger(t.Name(), t)
		rwc := debug.Wrap(l, c1)
		pkr1 = NewPacker(rwc)
	} else {
		pkr1 = NewPacker(c1)
	}
	pkr2 := NewPacker(c2)

	ctx := context.Background()

	pkt := codec.Packet{
		Req:  123,
		Flag: codec.FlagString | codec.FlagStream,
		Body: []byte("wat"),
	}

	errc := make(chan error)
	go func() {
		err := pkr1.w.WritePacket(&pkt)
		if err != nil {
			errc <- errors.Wrap(err, "failed to send test packet (wat)")
		}
		close(errc)
	}()

	var hdr codec.Header
	err := pkr2.NextHeader(ctx, &hdr)
	if err != nil {
		t.Fatal(err)
	}

	var buf = new(bytes.Buffer)
	err = pkr2.r.ReadBodyInto(buf, hdr.Len)
	if err != nil {
		t.Fatal(err)
	}

	// pkt.Req = -123
	var pkt_ = &codec.Packet{
		Flag: hdr.Flag,
		Req:  -hdr.Req,
		Body: buf.Bytes(),
	}

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

	// hdr, err := pkr2.NextHeader(ctx)

	err = pkr2.NextHeader(ctx, &hdr)
	if !luigi.IsEOS(err) {
		t.Fatal("expected EOS, got:", err)
	}

	err = pkr1.w.WritePacket(pkt_)
	if err == nil {
		t.Fatal("expected write-to-close-conn error, got nil")
	}

	sendErr, ok := <-errc
	if ok {
		t.Fatalf("%+v", sendErr)
	}

	t.Log("this error should be about pouring to a closed sink:", err)
}
