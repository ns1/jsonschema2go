package generate

import (
	"context"
	"github.com/jwilner/jsonschema2go/pkg/schema"
)

type Plan interface {
	Type() TypeInfo
	Deps() []TypeInfo
	ID() string
	Printable(imports *Imports) PrintablePlan
}

type Planner interface {
	Plan(ctx context.Context, helper Helper, schema *schema.Schema) (Plan, error)
}

type PrintablePlan interface {
	Template() string
	QualName(info TypeInfo) string
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
