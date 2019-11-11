package foo

// Bar gives you some dumb info
type Bar struct {
	Foo Foo `json:"foo,omitempty"`
}

type Foo struct {
	Name string `json:"name,omitempty"`
}
