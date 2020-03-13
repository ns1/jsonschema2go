package enum

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ns1/jsonschema2go/pkg/gen"
	"net/url"
)

//go:generate go run ../cmd/embedtmpl/embedtmpl.go enum enum.tmpl tmpl.gen.go

// Build attempts to generate an enum plan from a schema; if it cannot generate one, it returns ErrContinue.
func Build(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	if len(schema.Enum) == 0 {
		return nil, fmt.Errorf("no enum values: %w", gen.ErrContinue)
	}

	tInfo := helper.TypeInfoHinted(schema, schema.ChooseType())
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", gen.ErrContinue)
	}

	e := &Plan{TypeInfo: tInfo, ID: schema.ID}
	e.Comment = schema.Annotations.GetString("description")
	e.BaseType = gen.TypeInfo{Name: helper.Primitive(schema.ChooseType())}
	for _, m := range schema.Enum {
		name := helper.JSONPropertyExported(fmt.Sprintf("%s", m))
		e.Members = append(e.Members, Member{Name: name, Field: m})
	}
	return e, nil
}

// Execute generates the source code an enum from the given plan
func (e *Plan) Execute(imp *gen.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &enumPlanContext{imp, e})
	return w.String(), err
}

// Plan contains information about Enums to be rendered
type Plan struct {
	TypeInfo gen.TypeInfo
	ID       *url.URL

	Comment  string
	BaseType gen.TypeInfo
	Members  []Member
}

type enumPlanContext struct {
	*gen.Imports
	*Plan
}

// Member is an individual member of a enum
type Member struct {
	Name  string
	Field interface{}
}

// Type returns the TypeInfo for this enum
func (e *Plan) Type() gen.TypeInfo {
	return e.TypeInfo
}

// Deps returns any dependencies
func (e *Plan) Deps() []gen.TypeInfo {
	return []gen.TypeInfo{e.BaseType, {Name: "Sprintf", GoPath: "fmt"}}
}
