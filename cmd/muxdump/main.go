// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
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
	var req int
	var maxLen uint
	flag.IntVar(&req, "req", 0, "which request to filter out (default means all requests)")
	flag.UintVar(&maxLen, "maxlen", 256, "maximum length of a body, rest is truncated")
	flag.Parse()

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

		bodyBuf.Reset()
		err = rd.ReadBodyInto(bodyBuf, hdr.Len)
		if err != nil {
			if errors.Cause(err) == io.EOF {
				break
			}
			check(fmt.Errorf("failed to read body: %w", err))
			return
		}

		if req != 0 && int(hdr.Req) != req {
			continue
		}

		fmt.Printf("\n  [Req: %04d Size:%s %s\n", hdr.Req, humanize.Bytes(uint64(hdr.Len)), hdr.Flag)

		body := bodyBuf.Bytes()
		if uint(len(body)) > maxLen {
			body = append(body[:maxLen], '.', '.', '.')
		}

		var out string
		switch {
		case hdr.Flag.Get(codec.FlagJSON):
			out = string(body)
		default:
			out = hex.Dump(body)
		}
		fmt.Println(out)
	}

	fmt.Println("done")
}
