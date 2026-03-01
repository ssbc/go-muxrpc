// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"iter"
)

// SourceAs returns a Go 1.23 iterator that decodes each frame from the ByteSource as type T.
// Check src.Err() after the loop completes to see if iteration stopped due to an error.
func SourceAs[T any](ctx context.Context, src *ByteSource) iter.Seq[T] {
	return func(yield func(T) bool) {
		for b := range src.Iter(ctx) {
			var v T
			if err := json.Unmarshal(b, &v); err != nil {
				return
			}
			if !yield(v) {
				return
			}
		}
	}
}
