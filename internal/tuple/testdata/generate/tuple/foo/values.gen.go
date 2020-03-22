// Code generated by jsonschema2go. DO NOT EDIT.
package foo

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// Bar is generated from https://example.com/testdata/generate/tuple/foo/bar.json
// Bar gives you some dumb info
type Bar [2]interface{}

var (
	bar0Pattern = regexp.MustCompile(`^abcdef$`)
)

// Validate returns an error if this value is invalid according to rules defined in https://example.com/testdata/generate/tuple/foo/bar.json
func (t *Bar) Validate() error {
	if v, ok := m[0].(string); !ok {
		return &validationError{
			errType:  "type",
			path:     []interface{}{0},
			jsonPath: []interface{}{0},
			message:  fmt.Sprintf("must be string but got %T", m[0]),
		}
	} else if !bar0Pattern.MatchString(v) {
		return &validationError{
			errType:  "pattern",
			path:     []interface{}{0},
			jsonPath: []interface{}{0},
			message:  fmt.Sprintf(`must match '^abcdef$' but got %q`, v),
		}
	}
	if v, ok := m[1].(float64); !ok {
		return &validationError{
			errType:  "type",
			path:     []interface{}{1},
			jsonPath: []interface{}{1},
			message:  fmt.Sprintf("must be float64 but got %T", m[1]),
		}
	} else if v < 42.3 {
		return &validationError{
			errType:  "minimum",
			path:     []interface{}{1},
			jsonPath: []interface{}{1},
			message:  fmt.Sprintf("must be greater than or equal to 42.3 but was %v", v),
		}
	}
	return nil
}

func (t *Bar) UnmarshalJSON(data []byte) error {
	var msgs []json.RawMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return err
	}
	if len(msgs) > 0 {
		var item string
		if err := json.Unmarshal(msgs[0], &item); err != nil {
			return err
		}
		t[0] = item
	}
	if len(msgs) > 1 {
		var item float64
		if err := json.Unmarshal(msgs[1], &item); err != nil {
			return err
		}
		t[1] = item
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
