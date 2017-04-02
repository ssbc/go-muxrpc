package codec

import (
	"io"

	"github.com/go-kit/kit/log"
)

// Wrap decodes every packet that passes through it and logs it
func Wrap(l log.Logger, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	prout, pwout := io.Pipe()
	go func() {
		lout := log.With(l, "unit", "pipeFrom")
		r := NewReader(io.TeeReader(rwc, pwout))
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				lout.Log("error", err)
				pwout.CloseWithError(err)
				return
			}
			lout.Log("pkt", pkt)
		}
	}()

	prin, pwin := io.Pipe()
	w := NewWriter(rwc)
	go func() {
		lin := log.With(l, "unit", "pipeFrom")
		r := NewReader(prin)
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				if err != io.EOF {
					lin.Log("action", "ReadPacket", "error", err)
					prin.CloseWithError(err)
				}
				return
			}
			if err := w.WritePacket(pkt); err != nil {
				lin.Log("action", "WritePacket", "error", err)
				prin.CloseWithError(err)
				return
			}
			lin.Log("pkt", pkt)
		}
	}()
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{Reader: prout, Writer: pwin, Closer: w}
}
