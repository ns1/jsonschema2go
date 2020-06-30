package mapobj

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"unicode"

	"github.com/ns1/jsonschema2go/internal/validator"
	"github.com/ns1/jsonschema2go/pkg/gen"
)

//go:generate go run ../cmd/embedtmpl/embedtmpl.go mapobj map.tmpl map.gen.go

func PlanMap(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	if schema.ChooseType() != gen.JSONObject &&
		len(schema.Properties) != 0 ||
		schema.AdditionalProperties == nil ||
		(schema.AdditionalProperties.Schema == nil &&
			(schema.AdditionalProperties.Bool == nil || !*schema.AdditionalProperties.Bool)) {
		return nil, fmt.Errorf("not a object with only additional properties: %w", gen.ErrContinue)
	}

	typ, err := helper.TypeInfo(schema)
	if err != nil {
		return nil, err
	}

	var validators []validator.Validator
	valType := gen.TypeInfo{Name: "interface{}"}
	if schema.AdditionalProperties.Schema != nil {
		valSchema, err := schema.AdditionalProperties.Schema.Resolve(ctx, schema, helper)
		if err != nil {
			return nil, fmt.Errorf("unable to load addl property schema: %w", err)
		}
		valType, err = helper.TypeInfo(valSchema)
		if err != nil {
			return nil, err
		}
		if !valType.BuiltIn() {
			if err := helper.Dep(ctx, valSchema); err != nil {
				return nil, fmt.Errorf("unable to submit new dependency: %w", err)
			}
		}
		validators = validator.Validators(valSchema)
		validator.Sorted(validators)
	}

	m := &MapPlan{
		TypeInfo:      typ,
		ValTypeInfo:   valType,
		ID:            schema.ID,
		Comment:       schema.Annotations.GetString("description"),
		MinProperties: schema.MinProperties,
		Validators:    validators,
	}
	if schema.MaxProperties != nil {
		m.HasMaxProperties = true
		m.MaxProperties = *schema.MaxProperties
	}
	return m, nil
}

// MapPlan is an implementation of the interface Plan specific to structs
type MapPlan struct {
	TypeInfo         gen.TypeInfo
	ValTypeInfo      gen.TypeInfo
	ID               *url.URL
	MinProperties    uint64
	HasMaxProperties bool
	MaxProperties    uint64
	Validators       []validator.Validator
	Comment          string
}

func (m *MapPlan) Type() gen.TypeInfo {
	return m.TypeInfo
}

func (m *MapPlan) Deps() []gen.TypeInfo {
	deps := []gen.TypeInfo{m.ValTypeInfo}
	for _, v := range m.Validators {
		deps = append(deps, v.Deps...)
	}
	return deps
}

func (m *MapPlan) Execute(imports *gen.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &mapPlanContext{MapPlan: m, Imports: imports})
	return w.String(), err
}

type mapPlanContext struct {
	*MapPlan
	*gen.Imports
}

func (m *mapPlanContext) Comment() string {
	return gen.NormalizeComment(m.MapPlan.Comment)
}

func (m *mapPlanContext) ValidateInitialize() bool {
	for _, v := range m.Validators {
		if v.VarExpr != nil {
			return true
		}
	}
	return false
}

func (m *mapPlanContext) NameSpace() string {
	s := []rune(m.TypeInfo.Name)
	if len(s) > 0 {
		s[0] = unicode.ToLower(s[0])
	}
	return string(s)
}
