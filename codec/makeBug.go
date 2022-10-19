// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

//go:build ignore
// +build ignore

package main

import (
	"log"
	"os"

	"github.com/ssbc/go-muxrpc/v2/codec"
)

func main() {
	w := codec.NewWriter(os.Stdout)

	var pkts = []*codec.Packet{
		// {Flag: 0, Req: 0, Body: codec.Body("test1")},
		{Flag: codec.FlagStream | codec.FlagEndErr, Req: 1, Body: codec.Body("test2")},
		{Flag: codec.FlagStream | codec.FlagJSON, Req: 2, Body: codec.Body(`{"name": [ "hello" ], "args": [ "world" ]}`)},
	}
	for i, p := range pkts {
		err := w.WritePacket(p)
		log.Println(i, err)
	}
	err := w.Close()
	log.Println("c", err)
}
