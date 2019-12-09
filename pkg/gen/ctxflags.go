package gen

import "context"

// SetDebug sets a flag in the context indicating that debug mode has been enabled. The value may be accessed with
// IsDebug
func SetDebug(ctx context.Context) context.Context {
	return context.WithValue(ctx, debugCtxKey, true)
}

// IsDebug returns whether or not the debug flag has been set to true in this context.
func IsDebug(ctx context.Context) bool {
	b, ok := ctx.Value(debugCtxKey).(bool)
	return ok && b
}

type ctxKey int

const (
	debugCtxKey ctxKey = iota
)
