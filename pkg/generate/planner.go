package generate

import (
	"context"
	"github.com/jwilner/jsonschema2go/pkg/schema"
)

type Plan interface {
	Type() TypeInfo
	Deps() []TypeInfo
	ID() string
}

type Planner interface {
	Plan(ctx context.Context, helper Helper, schema *schema.Schema) (Plan, error)
}

type Helper interface {
	schema.Loader
	Schemas() <-chan *schema.Schema
	Dep(ctx context.Context, schemas ...*schema.Schema) error
	TypeInfo(s *schema.Schema) TypeInfo
	TypeInfoHinted(s *schema.Schema, t schema.SimpleType) TypeInfo
	JSONPropertyExported(name string) string
	Primitive(s schema.SimpleType) string
}
