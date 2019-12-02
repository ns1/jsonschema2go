// Code generated by jsonschema2go. DO NOT EDIT.
package foo

import (
	"fmt"
)

type A struct {
	B `json:"b,omitempty"`
}

func (m *A) Validate() error {
	if err := m.B.Validate(); err != nil {
		return err
	}
	return nil
}

type AValidationError struct {
	errType, jsonField, field, message string
}

func (e *AValidationError) ErrType() string {
	return e.errType
}

func (e *AValidationError) JSONField() string {
	return e.jsonField
}

func (e *AValidationError) Field() string {
	return e.field
}

func (e *AValidationError) Message() string {
	return e.message
}

func (e *AValidationError) Error() string {
	return fmt.Sprintf("%v: %v", e.field, e.message)
}