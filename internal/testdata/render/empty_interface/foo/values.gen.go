// Code generated by jsonschema2go. DO NOT EDIT.
package foo

import (
	"encoding/json"
	"fmt"
	"github.com/jwilner/jsonschema2go/boxed"
)

// Bar gives you some dumb info
type Bar struct {
	Name  boxed.String `json:"name"`
	Value interface{}  `json:"value,omitempty"`
}

func (m *Bar) Validate() error {
	return nil
}

func (m *Bar) MarshalJSON() ([]byte, error) {
	inner := struct {
		Name  *string     `json:"name,omitempty"`
		Value interface{} `json:"value,omitempty"`
	}{
		Value: m.Value,
	}
	if m.Name.Set {
		inner.Name = &m.Name.String
	}
	return json.Marshal(inner)
}

type BarValidationError struct {
	errType, jsonField, field, message string
}

func (e *BarValidationError) ErrType() string {
	return e.errType
}

func (e *BarValidationError) JSONField() string {
	return e.jsonField
}

func (e *BarValidationError) Field() string {
	return e.field
}

func (e *BarValidationError) Message() string {
	return e.message
}

func (e *BarValidationError) Error() string {
	return fmt.Sprintf("%v: %v", e.field, e.message)
}
