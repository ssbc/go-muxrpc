package codec

import (
	"io"

	"github.com/rs/xlog"
)

// Wrap decodes every packet that passes through it and logs it
func Wrap(rwc io.ReadWriteCloser) io.ReadWriteCloser {
	prout, pwout := io.Pipe()
	go func() {
		r := NewReader(io.TeeReader(rwc, pwout))
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				xlog.Error("From error:", err)
				pwout.CloseWithError(err)
				return
			}
			xlog.Info("From:", pkt)
		}
	}()

	prin, pwin := io.Pipe()
	w := NewWriter(rwc)
	go func() {
		r := NewReader(prin)
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				if err != io.EOF {
					xlog.Error("To error:", err)
					prin.CloseWithError(err)
				}
				return
			}
			if err := w.WritePacket(pkt); err != nil {
				xlog.Error("To error:", err)
				prin.CloseWithError(err)
				return
			}
			xlog.Info("To:", pkt)
		}
	}()
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{Reader: prout, Writer: pwin, Closer: w}
}
