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

package codec

import (
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
)

// Wrap decodes every packet that passes through it and logs it
func Wrap(l log.Logger, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	prout, pwout := io.Pipe()
	go func() {
		from := log.With(l, "unit", "pipeFrom")
		r := NewReader(io.TeeReader(rwc, pwout))
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				from.Log("error", err)
				//pwout.CloseWithError(err)
				return
			}
			from.Log("pkt", fmt.Sprintf("%+v", pkt))
		}
	}()

	prin, pwin := io.Pipe()
	w := NewWriter(rwc)
	go func() {
		to := log.With(l, "unit", "pipeTo")
		r := NewReader(prin)
		for {
			pkt, err := r.ReadPacket()
			if err != nil {
				if err != io.EOF {
					to.Log("action", "ReadPacket", "error", err)
					prin.CloseWithError(err)
				}
				return
			}
			if err := w.WritePacket(pkt); err != nil {
				to.Log("action", "WritePacket", "error", err)
				prin.CloseWithError(err)
				return
			}
			to.Log("pkt", fmt.Sprintf("%+v", pkt))
		}
	}()
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{Reader: prout, Writer: pwin, Closer: w}
}
