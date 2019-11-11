package foo

// Bar gives you some dumb info
type Bar struct {
	Inner Excluded `json:"inner,omitempty"`
	Name  string   `json:"name,omitempty"`
}
