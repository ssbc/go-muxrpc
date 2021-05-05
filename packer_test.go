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
		err := pkr1.w.WritePacket(pkt)
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

	var want = codec.Packet{
		Flag: hdr.Flag,
		Req:  -hdr.Req,
		Body: buf.Bytes(),
	}

	if !reflect.DeepEqual(want, pkt) {
		t.Log("Req matches:", reflect.DeepEqual(pkt.Req, want.Req))
		t.Log("Flag matches:", reflect.DeepEqual(pkt.Flag, want.Flag))
		t.Log("Body matches:", reflect.DeepEqual(pkt.Body, want.Body))
		t.Log("Packet matches:", reflect.DeepEqual(&pkt, want))
		t.Logf("expected packet %#v", &pkt)
		t.Logf("got %#v", want)
		t.Fatal("packet mismatch")
	}

	err = pkr1.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = pkr2.NextHeader(ctx, &hdr)
	if !errors.Is(err, io.EOF) {
		t.Fatal("expected EOF, got:", err)
	}

	err = pkr1.w.WritePacket(want)
	if err == nil {
		t.Fatal("expected write-to-close-conn error, got nil")
	}

	sendErr, ok := <-errc
	if ok {
		t.Fatalf("%+v", sendErr)
	}

	t.Log("this error should be about pouring to a closed sink:", err)
}
