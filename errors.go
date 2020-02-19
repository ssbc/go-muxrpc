package muxrpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrSessionTerminated is returned when the underlying connection died or rpc.Terminate() was called
var ErrSessionTerminated = errors.New("muxrpc: session terminated")

// CallError is returned when a call fails
type CallError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

func (e CallError) Error() string {
	return fmt.Sprintf("muxrpc CallError: %s - %s", e.Name, e.Message)
}

func parseError(data []byte) (CallError, error) {
	var e CallError

	err := json.Unmarshal(data, &e)
	if err != nil {
		return CallError{}, err
	}

	// There are also TypeErrors and numerous other things we might get from this..
	// if e.Name != "Error" {
	// 	return nil, errors.Errorf(`name is not "Error" but %q`, e.Name)
	// }

	return e, nil
}
