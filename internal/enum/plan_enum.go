package enum

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"github.com/jwilner/jsonschema2go/pkg/schema"
	"net/url"
)

//go:generate go run ../cmd/embedtmpl/embedtmpl.go enum enum.tmpl tmpl.gen.go
func Plan(ctx context.Context, helper generate.Helper, schema *schema.Schema) (generate.Plan, error) {
	if len(schema.Enum) == 0 {
		return nil, fmt.Errorf("no enum values: %w", generate.ErrContinue)
	}

	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", generate.ErrContinue)
	}

	e := &EnumPlan{TypeInfo: tInfo, id: schema.CalcID}
	e.Comment = schema.Annotations.GetString("description")
	e.BaseType = generate.TypeInfo{Name: helper.Primitive(schema.ChooseType())}
	for _, m := range schema.Enum {
		name := helper.JSONPropertyExported(fmt.Sprintf("%s", m))
		e.Members = append(e.Members, EnumMember{Name: name, Field: m})
	}
	return e, nil
}

type enumPlanContext struct {
	*generate.Imports
	*EnumPlan
}

func (e *EnumPlan) Execute(imp *generate.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &enumPlanContext{imp, e})
	return w.String(), err
}

type EnumPlan struct {
	TypeInfo generate.TypeInfo
	id       *url.URL

	Comment  string
	BaseType generate.TypeInfo
	Members  []EnumMember
}

func (e *EnumPlan) ID() string {
	if e.id != nil {
		return e.id.String()
	}
	return ""
}

type EnumMember struct {
	Name  string
	Field interface{}
}

func (e *EnumPlan) Type() generate.TypeInfo {
	return e.TypeInfo
}

func (e *EnumPlan) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{e.BaseType, {Name: "Sprintf", GoPath: "fmt"}}
}
