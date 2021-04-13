// SPDX-License-Identifier: MIT

package muxrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// TestPacker is a crappy golden path test
func TestPacker(t *testing.T) {
	c1, c2 := loPipe(t)

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
		err := pkr1.w.WritePacket(&pkt)
		if err != nil {
			errc <- fmt.Errorf("failed to send test packet (wat): %w", err)
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
	if !errors.Is(err, io.EOF) {
		t.Fatal("expected EOF, got:", err)
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
