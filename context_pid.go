package metadatax

import (
	"context"
	"errors"
)

var pidContextKey = contextKey{"process.pid"}

var PIDNotFoundError = errors.New("pid is not found in context")

func ContextWithPID(ctx context.Context, pid int32) context.Context {
	return context.WithValue(ctx, pidContextKey, pid)
}

func PIDFromContext(ctx context.Context) (int32, bool) {
	if pid, ok := ctx.Value(pidContextKey).(int32); ok {
		return pid, ok
	}

	return 0, false
}
