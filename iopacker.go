package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"fmt"
	"io"
	"sync"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"

	"github.com/pkg/errors"
)

var i int

type Packer interface {
	luigi.Source
	luigi.Sink
}

func NewIOPacker(rwc io.ReadWriteCloser) Packer {
	i++
	return &packer{
		r:  codec.NewReader(rwc),
		w:  codec.NewWriter(rwc),
		c:  rwc,
		id: i,
	}
}

type packer struct {
	rl sync.Mutex
	wl sync.Mutex

	r  *codec.Reader
	w  *codec.Writer
	c  io.Closer
	id int
}

func (pkr *packer) Next(ctx context.Context) (interface{}, error) {
	pkr.rl.Lock()
	defer pkr.rl.Unlock()

	pkt, err := pkr.r.ReadPacket()
	if errors.Cause(err) == io.EOF {
		return nil, luigi.EOS{}
	} else if err != nil {
		return nil, errors.Wrap(err, "ReadPacket failed.")
	}

	pkt.Req = -pkt.Req
	if pkt.Flag.Get(codec.FlagEndErr) {
		fmt.Printf("packer: received end/err on req %v\n", pkt.Req)
	}

	fmt.Println("packer.next:", i, pkt)
	return pkt, nil
}

func (pkr *packer) Pour(ctx context.Context, v interface{}) error {
	pkr.wl.Lock()
	defer pkr.wl.Unlock()

	pkt, ok := v.(*codec.Packet)
	if !ok {
		return errors.Errorf("packer sink expected type *codec.Packet, got %T", v)
	}

	return pkr.w.WritePacket(pkt)
}

func (pkr *packer) Close() error {
	pkr.wl.Lock()
	defer pkr.wl.Unlock()

	return pkr.c.Close()
}
