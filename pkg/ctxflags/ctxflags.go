package ctxflags

import "context"

func SetDebug(ctx context.Context) context.Context {
	return context.WithValue(ctx, debugCtxKey, true)
}

func IsDebug(ctx context.Context) bool {
	b, ok := ctx.Value(debugCtxKey).(bool)
	return ok && b
}

type ctxKey int

const (
	debugCtxKey ctxKey = iota
)
