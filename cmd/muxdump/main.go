package main

import (
	"fmt"
	"io"
	"os"

	"github.com/cryptix/go/logging"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

var check = logging.CheckFatal

func main() {
	rd := codec.NewReader(os.Stdin)

	for {

		pkt, err := rd.ReadPacket()
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			}
			check(err)
			return
		}

		fmt.Printf("\n  [Req: %04d Size:%s %s\n", pkt.Req, humanize.Bytes(uint64(len(pkt.Body))), pkt.Flag)
		if len(pkt.Body) > 128 {
			pkt.Body = pkt.Body[:128]
		}
		fmt.Println(string(pkt.Body))
	}
	fmt.Println("done")
}
