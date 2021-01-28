package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

func check(err error) {
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func main() {
	rd := codec.NewReader(os.Stdin)

	var (
		hdr     codec.Header
		bodyBuf = new(bytes.Buffer)
	)

	for {
		err := rd.ReadHeader(&hdr)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			}
			check(fmt.Errorf("failed to read header: %w", err))
			return
		}

		fmt.Printf("\n  [Req: %04d Size:%s %s\n", hdr.Req, humanize.Bytes(uint64(hdr.Len)), hdr.Flag)

		err = rd.ReadBodyInto(bodyBuf, hdr.Len)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			}
			check(fmt.Errorf("failed to read body: %w", err))
			return
		}
		body := bodyBuf.Bytes()

		if hdr.Flag.Get(codec.FlagString) || hdr.Flag.Get(codec.FlagJSON) {
			if len(body) > 250 {
				body = body[:250]
			}
			fmt.Println(string(body))
		} else {
			if len(body) > 32 {
				body = body[:32]
			}
			fmt.Printf("%x\n", body)
		}

		bodyBuf.Reset()

	}
	fmt.Println("done")
}
