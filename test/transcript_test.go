// SPDX-License-Identifier: MIT

package test

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"
)

func TestTranscript(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	var ts Transcript

	conn1, conn2 := net.Pipe()

	rwc1 := Wrap(&ts, conn1)

	pkr1 := muxrpc.NewPacker(rwc1)
	pkr2 := muxrpc.NewPacker(conn2)

	wait := make(chan struct{})

	go func() {
		iPkt1, err := pkr2.Next(context.TODO())
		r.NoError(err, "error reading packet")

		pkt1 := iPkt1.(*codec.Packet)
		a.Equal(codec.FlagStream, pkt1.Flag, "packet flag mismatch")
		a.Equal(int32(-1), pkt1.Req, "packet req mismatch")
		a.Equal(codec.Body("Hello Hermies!"), pkt1.Body, "packate body mismatch")

		_, err = pkr2.Next(context.TODO())
		wait <- struct{}{}
		defer func() { wait <- struct{}{} }()
		r.EqualError(err, (luigi.EOS{}).Error())
	}()

	err := pkr1.Pour(context.TODO(), &codec.Packet{
		Flag: codec.FlagStream,
		Req:  1,
		Body: []byte("Hello Hermies!"),
	})
	r.NoError(err, "error pouring packet")

	err = pkr1.Close()
	r.NoError(err, "error closing packer")

	<-wait

	pkts := ts.Get()
	a.Len(pkts, 3, "mismatched amount of read packets")

	pktSpecMsg := MergePacketSpec(
		BodyPacketSpec(EqualBodySpec(codec.Body("Hello Hermies!"))),
		FlagEqualPacketSpec(codec.FlagStream),
		ReqPacketSpec(1),
		DirPacketSpec(DirOut),
	)

	pktSpecErrIn := MergePacketSpec(
		DirPacketSpec(DirIn),
		ErrorPacketSpec(io.EOF.Error()),
	)

	pktSpecErrOut := MergePacketSpec(
		DirPacketSpec(DirOut),
		ErrorPacketSpec(io.EOF.Error()),
	)

	tsSpec := MergeTranscriptSpec(
		UniqueMatchTranscriptSpec(pktSpecMsg),
		UniqueMatchTranscriptSpec(pktSpecErrIn),
		UniqueMatchTranscriptSpec(pktSpecErrOut),
		OrderTranscriptSpec(pktSpecMsg, pktSpecErrIn),
		OrderTranscriptSpec(pktSpecMsg, pktSpecErrOut),
	)

	var rrr DoesErrorer

	tsSpec(&rrr, &ts)
	if !rrr {
		t.Error("unexpected transcript")
	}

	t.Log(pkts)

	<-wait
}
