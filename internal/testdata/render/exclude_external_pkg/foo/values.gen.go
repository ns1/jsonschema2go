package foo

import (
	"github.com/jwilner/json2go/internal/testdata/render/exclude_external_pkg/other"
)

// Bar gives you some dumb info
type Bar struct {
	Inner other.Excluded `json:"inner,omitempty"`
	Name  string         `json:"name,omitempty"`
}
