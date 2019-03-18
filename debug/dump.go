package debug

import (
	"io"
	"net"
	"os"

	"github.com/cryptix/go/logging"
)

// Dump decodes every packet that passes through it and logs it
// TODO: add timestmaps to individual packets
// TODO: maybe make it one file - something like pcap would be rat but it's also quite the beast
func Dump(name string, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	rx, err := os.Create(name + ".rx")
	logging.CheckFatal(err)
	tx, err := os.Create(name + ".tx")
	logging.CheckFatal(err)
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: io.TeeReader(rwc, rx),
		Writer: io.MultiWriter(rwc, tx),
		Closer: closer(func() error {
			rx.Close()
			tx.Close()
			return rwc.Close()
		}),
	}

}

func WrapDump(c net.Conn) (net.Conn, error) {
	return &wrappedConn{c, Dump(c.RemoteAddr().String(), c)}, nil
}
