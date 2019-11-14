package foo

type Bar struct {
	Options []Inner `json:"options,omitempty"`
}

type Inner struct {
	Name  string      `json:"name,omitempty"`
	Value interface{} `json:"value,omitempty"`
}
