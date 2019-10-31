package foo

// Bar gives you some dumb info
type Bar struct {
	Name string `json:"name,omitempty"`
	Blob
}

type Blob struct {
	Count int `json:"count,omitempty"`
}
