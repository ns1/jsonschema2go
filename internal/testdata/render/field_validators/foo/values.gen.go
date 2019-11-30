// Code generated by jsonschema2go. DO NOT EDIT.
package foo

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// Bar gives you some dumb info
// generated from https://example.com/testdata/render/field_validators/foo/bar.json
type Bar struct {
	Array       BarArray `json:"array,omitempty"`
	ExclInteger int      `json:"exclInteger,omitempty"`
	ExclNumber  float64  `json:"exclNumber,omitempty"`
	Integer     int      `json:"integer,omitempty"`
	Number      float64  `json:"number,omitempty"`
	String      string   `json:"string,omitempty"`
}

var (
	barStringPattern = regexp.MustCompile(`^(123|456)$`)
)

func (m *Bar) Validate() error {
	if err := m.Array.Validate(); err != nil {
		return err
	}
	if m.ExclInteger >= 1 {
		return &BarValidationError{
			errType:   "maximumExclusive",
			jsonField: "exclInteger",
			field:     "ExclInteger",
			message:   fmt.Sprintf("must be less than 1 but was %v", m.ExclInteger),
		}
	}
	if m.ExclInteger <= 1 {
		return &BarValidationError{
			errType:   "minimumExclusive",
			jsonField: "exclInteger",
			field:     "ExclInteger",
			message:   fmt.Sprintf("must be greater than 1 but was %v", m.ExclInteger),
		}
	}
	if m.ExclNumber >= 1 {
		return &BarValidationError{
			errType:   "maximumExclusive",
			jsonField: "exclNumber",
			field:     "ExclNumber",
			message:   fmt.Sprintf("must be less than 1 but was %v", m.ExclNumber),
		}
	}
	if m.ExclNumber <= 1 {
		return &BarValidationError{
			errType:   "minimumExclusive",
			jsonField: "exclNumber",
			field:     "ExclNumber",
			message:   fmt.Sprintf("must be greater than 1 but was %v", m.ExclNumber),
		}
	}
	if m.Integer > 1 {
		return &BarValidationError{
			errType:   "maximum",
			jsonField: "integer",
			field:     "Integer",
			message:   fmt.Sprintf("must be less than or equal to 1 but was %v", m.Integer),
		}
	}
	if m.Integer < 1 {
		return &BarValidationError{
			errType:   "minimum",
			jsonField: "integer",
			field:     "Integer",
			message:   fmt.Sprintf("must be greater than or equal to 1 but was %v", m.Integer),
		}
	}
	if m.Integer%3 != 0 {
		return &BarValidationError{
			errType:   "multipleOf",
			jsonField: "integer",
			field:     "Integer",
			message:   fmt.Sprintf("must be a multiple of 3 but was %d", m.Integer),
		}
	}
	if m.Number > 1 {
		return &BarValidationError{
			errType:   "maximum",
			jsonField: "number",
			field:     "Number",
			message:   fmt.Sprintf("must be less than or equal to 1 but was %v", m.Number),
		}
	}
	if m.Number < 1 {
		return &BarValidationError{
			errType:   "minimum",
			jsonField: "number",
			field:     "Number",
			message:   fmt.Sprintf("must be greater than or equal to 1 but was %v", m.Number),
		}
	}
	if len(m.String) > 10 {
		return &BarValidationError{
			errType:   "maxLength",
			jsonField: "string",
			field:     "String",
			message:   fmt.Sprintf("must have length less than 10 but was %d", len(m.String)),
		}
	}
	if len(m.String) < 3 {
		return &BarValidationError{
			errType:   "minLength",
			jsonField: "string",
			field:     "String",
			message:   fmt.Sprintf("must have length greater than 3 but was %d", len(m.String)),
		}
	}
	if !barStringPattern.MatchString(m.String) {
		return &BarValidationError{
			errType:   "pattern",
			jsonField: "string",
			field:     "String",
			message:   fmt.Sprintf("must match '^(123|456)$' but got %q", m.String),
		}
	}
	return nil
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

// generated from https://example.com/testdata/render/field_validators/foo/bar.json#/properties/array
type BarArray []string

func (m BarArray) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal([]string(m))
}

func (m BarArray) Validate() error {
	if len(m) > 10 {
		return &BarArrayValidationError{
			"maxItems",
			"",
			"",
			fmt.Sprintf("must have length greater than 10 but was %d", len(m)),
		}
	}
	if len(m) < 1 {
		return &BarArrayValidationError{
			"minItems",
			"",
			"",
			fmt.Sprintf("must have length greater than 1 but was %d", len(m)),
		}
	}
	seen := make(map[string]bool)
	for _, v := range m {
		if seen[v] {
			return &BarArrayValidationError{
				errType:   "uniqueItems",
				jsonField: "",
				field:     "",
				message:   fmt.Sprintf("items must be unique but %v occurs more than once", v),
			}
		}
		seen[v] = true
	}
	return nil
}

type BarArrayValidationError struct {
	errType, jsonField, field, message string
}

func (e *BarArrayValidationError) ErrType() string {
	return e.errType
}

func (e *BarArrayValidationError) JSONField() string {
	return e.jsonField
}

func (e *BarArrayValidationError) Field() string {
	return e.field
}

func (e *BarArrayValidationError) Message() string {
	return e.message
}

func (e *BarArrayValidationError) Error() string {
	return fmt.Sprintf("%v: %v", e.field, e.message)
}
