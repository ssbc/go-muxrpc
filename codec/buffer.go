// SPDX-FileCopyrightText: 2022 Luke Murphy
//
// SPDX-License-Identifier: MIT

//go:build !(386 || arm)
// +build !386,!arm

package codec

import (
	"math"
)

const maxBufferSize = math.MaxUint32
