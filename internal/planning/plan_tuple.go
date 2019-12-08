package planning

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"net/url"
	"unicode"
)

type TuplePlan struct {
	typeInfo generate.TypeInfo
	id       *url.URL
	Comment  string

	Items []*TupleItem
}

func (t *TuplePlan) Printable(imports *generate.Imports) generate.PrintablePlan {
	return &TuplePlanContext{Imports: imports, TuplePlan: t}
}

func (t *TuplePlan) ArrayLength() int {
	return len(t.Items)
}

func (t *TuplePlan) Type() generate.TypeInfo {
	return t.typeInfo
}

func (t *TuplePlan) Deps() []generate.TypeInfo {
	deps := []generate.TypeInfo{
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
			if v.varExpr != nil {
				return true
			}
		}
	}
	return false
}

type TupleItem struct {
	Comment    string
	Type       generate.TypeInfo
	validators []Validator
}

func (t TupleItem) Validators() []Validator {
	return sortedValidators(t.validators)
}

func PlanTuple(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	if schema.ChooseType() != sch.Array {
		return nil, fmt.Errorf("not an array: %w", ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", ErrContinue)
	}
	if schema.Items == nil || len(schema.Items.TupleFields) == 0 {
		return nil, fmt.Errorf("not a tuple: %w", ErrContinue)
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
				validators: []Validator{SubschemaValidator},
			})
			continue
		}
		if !t.BuiltIn() {
			if err := helper.Dep(ctx, s); err != nil {
				return nil, err
			}
		}
		vals := validators(s)
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
	*generate.Imports
	*TuplePlan
}

func (t *TuplePlanContext) Template() string {
	return "tuple.tmpl"
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
