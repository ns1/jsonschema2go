package foo

import (
	"encoding/json"
)

// Bar gives you some dumb info
type Bar struct {
	Name *string `json:"name,omitempty"`
}

// Barz gives you lots of dumb info
type Barz []Bar

func (m Barz) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal([]Bar(m))
}
