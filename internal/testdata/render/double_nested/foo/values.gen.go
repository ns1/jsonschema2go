package foo

// Bar gives you some dumb info
type Bar struct {
	Foo Foo `json:"foo,omitempty"`
}

type Baz struct {
	Name string `json:"name,omitempty"`
}

type Foo struct {
	Baz Baz `json:"baz,omitempty"`
}
