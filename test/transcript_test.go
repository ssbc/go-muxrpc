/*
This file is part of go-muxrpc.

go-muxrpc is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

go-muxrpc is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with go-muxrpc.  If not, see <http://www.gnu.org/licenses/>.
*/

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

	if !tsSpec(&ts) {
		t.Error("unexpected transcript")
	}

	t.Log(pkts)

	<-wait
}
