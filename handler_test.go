// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

//go:build ignore
// +build ignore

package muxrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-muxrpc/v2/codec"
)

func TestHandlerMux(t *testing.T) {
	mux := &HandlerMux{}
	r := require.New(t)

	var wg sync.WaitGroup
	wg.Add(3)

	todo := context.TODO()

	var b1 = &bytes.Buffer{}
	var w1 = codec.NewWriter(b1)

	var b2 = &bytes.Buffer{}
	var w2 = codec.NewWriter(b2)

	exp := &Request{
		Method: Method{"foo", "bar"},
		Stream: newStream(nil, w1, 1, streamCapMultiple, streamCapMultiple),
	}
	notexp := &Request{
		Method: Method{"goo", "bar"},
		Stream: newStream(nil, w2, 2, streamCapMultiple, streamCapMultiple),
	}

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, edp Endpoint) {
		if exp.Method.String() == req.Method.String() {
			req.Stream.Close()
		} else {
			req.Stream.CloseWithError(fmt.Errorf("test failed"))
		}
		wg.Done()
	})
	fh.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		fmt.Println("mux: h connected")
		wg.Done()
	})

	mux.Register(Method{"foo", "bar"}, &fh)

	go func() {
		mux.HandleConnect(todo, nil)
		fmt.Println("mux: ran connect")

		mux.HandleCall(todo, notexp, nil)
		fmt.Println("mux: sent notexp")
		mux.HandleCall(todo, exp, nil)
		fmt.Println("mux: sent exp")
		wg.Done()
	}()

	wg.Wait() // checking

	var endHdr codec.Header
	var err error

	rd2 := codec.NewReader(b2)
	err = rd2.ReadHeader(&endHdr)
	r.NoError(err)
	r.NotEqual(uint32(4), endHdr.Len, "should not be a plain true")

	body, err := ioutil.ReadAll(rd2.NextBodyReader(endHdr.Len))
	r.NoError(err)
	var actualErr CallError
	r.NoError(json.Unmarshal(body, &actualErr))
	r.Equal("no such command: goo.bar", actualErr.Message)

	// vEndOK, err := src1.Next(todo)
	// r.Error(luigi.EOS{}, err)

	rd1 := codec.NewReader(b1)

	err = rd1.ReadHeader(&endHdr)
	r.NoError(err)
	r.True(endHdr.Flag.Get(codec.FlagEndErr))
	r.Equal(uint32(4), endHdr.Len, "should not be a plain true")

	body, err = ioutil.ReadAll(rd1.NextBodyReader(endHdr.Len))
	r.NoError(err)
	r.Equal([]byte("true"), body)

}
