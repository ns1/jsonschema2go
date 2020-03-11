package gen

type GoBaseType int

const (
	GoUnknown GoBaseType = iota
	GoBool
	GoInt64
	GoFloat64
	GoString
	GoEmpty
	GoSlice
	GoArray
	GoMap
	GoStruct
)
