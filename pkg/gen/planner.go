package gen

import (
	"context"
	"errors"
)

// ErrContinue can be returned from a Planner if there is no match. Other planners may then be tried.
var ErrContinue = errors.New("continue")

// Plan is the contract that must be filled for a type to be rendered.
type Plan interface {
	// Type returns the TypeInfo for the current type
	Type() TypeInfo
	// Deps returns any dependent types for the current type (i.e. any types requiring import)
	Deps() []TypeInfo
	// Execute is provided a resolved imports and should provide the type rendered as a string.
	Execute(imports *Imports) (string, error)
}

// Planner is a strategy for generating a Plan from a Schema
type Planner interface {
	// Plan generates a Plan from a Schema. If the error matches `errors.Is(err, ErrContinue)`, processing may continue.
	Plan(ctx context.Context, helper Helper, schema *Schema) (Plan, error)
}

// Helper is an interface provided to each planner
type Helper interface {
	Loader
	Dep(ctx context.Context, schemas ...*Schema) error
	TypeInfo(s *Schema) (TypeInfo, error)
	ErrSimpleTypeUnknown(err error) bool
	DetectSimpleType(ctx context.Context, s *Schema) (JSONType, error)
	DetectGoBaseType(ctx context.Context, s *Schema) (GoBaseType, error)
	TypeInfoHinted(s *Schema, t JSONType) TypeInfo
	JSONPropertyExported(name string) string
	Primitive(s JSONType) string
}
