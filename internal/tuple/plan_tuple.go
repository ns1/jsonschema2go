package tuple

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/validator"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"net/url"
	"unicode"
)

type TuplePlan struct {
	typeInfo gen.TypeInfo
	id       *url.URL
	Comment  string

	Items []*TupleItem
}

//go:generatego run ../cmd/embedtmpl/embedtmpl.go tuple tuple.tmpl tmpl.gen.go
func (t *TuplePlan) Execute(imports *gen.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &TuplePlanContext{imports, t})
	return w.String(), err
}

func (t *TuplePlan) ArrayLength() int {
	return len(t.Items)
}

func (t *TuplePlan) Type() gen.TypeInfo {
	return t.typeInfo
}

func (t *TuplePlan) Deps() []gen.TypeInfo {
	deps := []gen.TypeInfo{
		{GoPath: "encoding/json", Name: "Marshal"},
		{GoPath: "encoding/json", Name: "Unmarshal"},
		{GoPath: "fmt", Name: "Sprintf"},
	}
	for _, f := range t.Items {
		deps = append(deps, f.Type)
		for _, v := range f.validators {
			deps = append(deps, v.Deps...)
		}
	}
	return deps
}

func (t *TuplePlan) ID() string {
	if t.id != nil {
		return t.id.String()
	}
	return ""
}

func (t *TuplePlan) ValidateInitialize() bool {
	for _, f := range t.Items {
		for _, v := range f.validators {
			if v.VarExpr != nil {
				return true
			}
		}
	}
	return false
}

type TupleItem struct {
	Comment    string
	Type       gen.TypeInfo
	validators []validator.Validator
}

func (t TupleItem) Validators() []validator.Validator {
	return validator.Sorted(t.validators)
}

func PlanTuple(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	if schema.ChooseType() != gen.Array {
		return nil, fmt.Errorf("not an array: %w", gen.ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", gen.ErrContinue)
	}
	if schema.Items == nil || len(schema.Items.TupleFields) == 0 {
		return nil, fmt.Errorf("not a tuple: %w", gen.ErrContinue)
	}
	_, schemas, err := loadSchemaList(ctx, helper, schema, schema.Items.TupleFields)
	if err != nil {
		return nil, err
	}

	var items []*TupleItem
	for _, s := range schemas {
		t := helper.TypeInfo(s)
		if t.Unknown() {
			t.Name = "interface{}"
			items = append(items, &TupleItem{
				Comment:    s.Annotations.GetString("description"),
				Type:       t,
				validators: []validator.Validator{validator.SubschemaValidator},
			})
			continue
		}
		if !t.BuiltIn() {
			if err := helper.Dep(ctx, s); err != nil {
				return nil, err
			}
		}
		vals := validator.Validators(s)
		items = append(items, &TupleItem{
			Comment:    s.Annotations.GetString("description"),
			Type:       t,
			validators: vals,
		})
	}

	return &TuplePlan{
		typeInfo: tInfo,
		Comment:  schema.Annotations.GetString("description"),
		id:       schema.CalcID,
		Items:    items,
	}, nil
}

type TuplePlanContext struct {
	*gen.Imports
	*TuplePlan
}

type EnrichedTupleItem struct {
	TuplePlan *TuplePlanContext
	idx       int
	*TupleItem
}

func (e *EnrichedTupleItem) NameSpace() string {
	name := fmt.Sprintf("%s%d", e.TuplePlan.Type().Name, e.idx)
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

func (t *TuplePlanContext) Items() (items []*EnrichedTupleItem) {
	for idx, item := range t.TuplePlan.Items {
		items = append(items, &EnrichedTupleItem{t, idx, item})
	}
	return
}

func loadSchemaList(
	ctx context.Context,
	helper gen.Helper,
	parent *gen.Schema,
	schemas []*gen.RefOrSchema,
) (gen.SimpleType, []*gen.Schema, error) {
	var (
		resolved  []*gen.Schema
		foundType gen.SimpleType
	)
	for _, s := range schemas {
		r, err := s.Resolve(ctx, parent, helper)
		if err != nil {
			return gen.Unknown, nil, err
		}
		resolved = append(resolved, r)
		t := r.ChooseType()
		if t == gen.Unknown {
			continue
		}
		if foundType == gen.Unknown {
			foundType = t
			continue
		}
	}
	return foundType, resolved, nil
}
