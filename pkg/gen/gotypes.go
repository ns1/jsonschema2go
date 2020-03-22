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

func (g GoBaseType) ReferenceType() bool {
	return g == GoSlice || g == GoMap
}

func (g GoBaseType) ScalarType() bool {
	return g >= GoBool && g < GoEmpty
}