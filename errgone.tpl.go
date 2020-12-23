// +build ignore

package muxrpc

import (
	"fmt"

	"github.com/pkg/errors"
)

func before(err error, msg string) error {
	return errors.Wrap(err, msg)
}

func after(err error, msg string) error {
	return fmt.Errorf(msg+": %w", err)
}
