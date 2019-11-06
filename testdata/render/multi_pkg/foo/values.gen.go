package foo

import (
	"example.com/v1/foobar"
)

// Bar gives you some dumb info
type Bar struct {
	Name string `json:"name,omitempty"`
	foobar.Blob
}
