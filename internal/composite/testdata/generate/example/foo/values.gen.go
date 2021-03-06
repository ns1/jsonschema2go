// Code generated by jsonschema2go. DO NOT EDIT.
package foo

import (
	"fmt"
	"regexp"
)

// Bar is generated from https://example.com/testdata/generate/example/foo/bar.json
// Bar contains some info
type Bar struct {
	Baz   *string `json:"baz,omitempty"`
	Count *int64  `json:"count,omitempty"`
}

var (
	barBazPattern = regexp.MustCompile(`^[0-9a-fA-F]{10}$`)
)

// Validate returns an error if this value is invalid according to rules defined in https://example.com/testdata/generate/example/foo/bar.json
func (m *Bar) Validate() error {
	if m.Baz == nil {
		return &validationError{
			errType:  "required",
			message:  "field required",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
		}
	}
	if !barBazPattern.MatchString(*m.Baz) {
		return &validationError{
			errType:  "pattern",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
			message:  fmt.Sprintf(`must match '^[0-9a-fA-F]{10}$' but got %q`, *m.Baz),
		}
	}
	if m.Count != nil && *m.Count < 3 {
		return &validationError{
			errType:  "minimum",
			path:     []interface{}{"Count"},
			jsonPath: []interface{}{"count"},
			message:  fmt.Sprintf("must be greater than or equal to 3 but was %v", *m.Count),
		}
	}
	return nil
}

type valErr interface {
	ErrType() string
	JSONPath() []interface{}
	Path() []interface{}
	Message() string
}

type validationError struct {
	errType, message string
	jsonPath, path   []interface{}
}

func (e *validationError) ErrType() string {
	return e.errType
}

func (e *validationError) JSONPath() []interface{} {
	return e.jsonPath
}

func (e *validationError) Path() []interface{} {
	return e.path
}

func (e *validationError) Message() string {
	return e.message
}

func (e *validationError) Error() string {
	return fmt.Sprintf("%v: %v", e.path, e.message)
}

var _ valErr = new(validationError)
