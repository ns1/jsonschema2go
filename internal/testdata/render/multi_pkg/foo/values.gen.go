package foo

import (
	"github.com/jwilner/jsonschema2go/internal/testdata/render/multi_pkg/foobar"
)

// Bar gives you some dumb info
type Bar struct {
	Name string `json:"name,omitempty"`
	foobar.Blob
}
