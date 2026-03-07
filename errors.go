// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package muxrpc

import (
	"encoding/json"
	"errors"
	stderr "errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
)

// ErrSessionTerminated is returned once Terminate() was called  or the connection dies
var ErrSessionTerminated = errors.New("muxrpc: session terminated")

var errSinkClosed = stderr.New("muxrpc: pour to closed sink")

type ErrNoSuchMethod struct {
	Method Method
}

func (e ErrNoSuchMethod) Error() string {
	return fmt.Sprintf("muxrpc: no such command: %s", e.Method)
}

// CallError is returned when a call fails
type CallError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

func (e CallError) Error() string {
	return fmt.Sprintf("muxrpc CallError: %s - %s", e.Name, e.Message)
}

func parseError(data []byte) (*CallError, error) {
	var e CallError

	err := json.Unmarshal(data, &e)
	if err != nil {
		return nil, fmt.Errorf("muxrpc: failed to unmarshal error packet: %w", err)
	}

	// There are also TypeErrors and numerous other things we might get from this..
	// if e.Name != "Error" {
	// 	return nil, fmt.Errorf(`name is not "Error" but %q`, e.Name)
	// }

	return &e, nil
}

type ErrWrongStreamType struct{ ct CallType }

func (wst ErrWrongStreamType) Error() string {
	return fmt.Sprintf("muxrpc: wrong stream type: %s", wst.ct)
}

// IsSinkClosed checks if the error indicates a closed sink
func IsSinkClosed(err error) bool {
	if err == nil {
		return false
	}

	if err == errSinkClosed {
		return true
	}

	if err == ErrSessionTerminated {
		return true
	}

	if isAlreadyClosed(err) {
		return true
	}

	var ce *CallError
	if errors.As(err, &ce) && ce.Message == "unexpected end of parent stream" {
		return true
	}

	return false
}

func isAlreadyClosed(err error) bool {
	if err == nil {
		return false
	}

	if stderr.Is(err, io.EOF) || stderr.Is(err, os.ErrClosed) || stderr.Is(err, io.ErrClosedPipe) || stderr.Is(err, net.ErrClosed) {
		return true
	}

	var sysErr *os.PathError
	if stderr.As(err, &sysErr) {
		if sysErr.Err == os.ErrClosed {
			return true
		}
	}

	var opErr *net.OpError
	if stderr.As(err, &opErr) {
		var syscallErr *os.SyscallError
		if stderr.As(opErr.Err, &syscallErr) {
			if errNo, ok := syscallErr.Err.(syscall.Errno); ok {
				if errNo == syscall.EPIPE || errNo == syscall.ECONNRESET {
					return true
				}
			}
		}
	}
	return false
}
