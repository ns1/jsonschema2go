package boxed

import (
	"encoding/json"
	"errors"
)

var ErrMarshalUnset = errors.New("marshalling unset var")

type Int64 struct {
	Int64 int64
	Set   bool
}

func (m Int64) MarshalJSON() ([]byte, error) {
	if !m.Set {
		return nil, ErrMarshalUnset
	}
	return json.Marshal(m.Int64)
}

func (m *Int64) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Int64)
}

type Float64 struct {
	Float64 float64
	Set     bool
}

func (m Float64) MarshalJSON() ([]byte, error) {
	if !m.Set {
		return nil, ErrMarshalUnset
	}
	return json.Marshal(m.Float64)
}

func (m *Float64) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Float64)
}

type String struct {
	String string
	Set    bool
}

func (m String) MarshalJSON() ([]byte, error) {
	if !m.Set {
		return nil, ErrMarshalUnset
	}
	return json.Marshal(m.String)
}

func (m *String) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.String)
}

type Bool struct {
	Bool bool
	Set  bool
}

func (m Bool) MarshalJSON() ([]byte, error) {
	if !m.Set {
		return nil, ErrMarshalUnset
	}
	return json.Marshal(m.Bool)
}

func (m *Bool) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Bool)
}
