package boxed

import (
	"encoding/json"
	"errors"
)

var (
	ErrMarshalUnset = errors.New("marshalling unset var")
	ErrNullInvalid  = errors.New("null is invalid")
)

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
	if string(data) == "null" {
		return ErrNullInvalid
	}
	if err := json.Unmarshal(data, &m.Int64); err != nil {
		return err
	}
	m.Set = true
	return nil
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
	if string(data) == "null" {
		return ErrNullInvalid
	}
	if err := json.Unmarshal(data, &m.Float64); err != nil {
		return err
	}
	m.Set = true
	return nil
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
	if string(data) == "null" {
		return ErrNullInvalid
	}
	if err := json.Unmarshal(data, &m.String); err != nil {
		return err
	}
	m.Set = true
	return nil
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
	if string(data) == "null" {
		return ErrNullInvalid
	}
	if err := json.Unmarshal(data, &m.Bool); err != nil {
		return err
	}
	m.Set = true
	return nil
}
