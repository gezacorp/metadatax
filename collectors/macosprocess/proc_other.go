//go:build !darwin || !cgo

package macosprocess

import (
	"context"
	"errors"
)

func inspect(context.Context, int32) (ProcessInfo, error) {
	return ProcessInfo{}, errors.ErrUnsupported
}
