package planning

import (
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"net/url"
	"sort"
	"strconv"
)

type SlicePlan struct {
	TypeInfo generate.TypeInfo
	id       *url.URL

	Comment        string
	ItemType       generate.TypeInfo
	validators     []Validator
	itemValidators []Validator
}

func (a *SlicePlan) ID() string {
	if a.id != nil {
		return a.id.String()
	}
	return ""
}

func (a *SlicePlan) Type() generate.TypeInfo {
	return a.TypeInfo
}

func (a *SlicePlan) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{a.ItemType, {Name: "Marshal", GoPath: "encoding/json"}, {Name: "Sprintf", GoPath: "fmt"}}
}

func (a *SlicePlan) Validators() []Validator {
	sort.Slice(a.validators, func(i, j int) bool {
		return a.validators[i].Name < a.validators[j].Name
	})
	return a.validators
}

func (a *SlicePlan) ItemValidators() []Validator {
	sort.Slice(a.itemValidators, func(i, j int) bool {
		return a.itemValidators[i].Name < a.itemValidators[j].Name
	})
	return a.itemValidators
}

func (a *SlicePlan) ItemValidateInitialize() bool {
	for _, i := range a.itemValidators {
		if i.varExpr != nil {
			return true
		}
	}
	return false
}

func planSlice(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	if schema.ChooseType() != sch.Array {
		return nil, fmt.Errorf("not an array: %w", ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", ErrContinue)
	}

	// we've matched
	var itemSchema *sch.Schema
	if schema.Items != nil && schema.Items.Items != nil {
		var err error
		if itemSchema, err = schema.Items.Items.Resolve(ctx, schema, helper); err != nil {
			return nil, err
		}
	}
	a := SlicePlan{TypeInfo: tInfo, id: schema.CalcID}
	a.Comment = schema.Annotations.GetString("description")
	if itemSchema != nil {
		if a.ItemType = helper.TypeInfo(itemSchema); a.ItemType.Unknown() {
			return nil, nil
		}
	} else {
		a.ItemType = generate.TypeInfo{Name: "interface{}"}
	}
	if !a.ItemType.BuiltIn() && itemSchema != nil {
		if err := helper.Dep(ctx, itemSchema); err != nil {
			return nil, err
		}
	}
	if schema.MinItems > 0 {
		minItemsS := strconv.FormatUint(schema.MinItems, 10)
		a.validators = append(a.validators, Validator{
			Name:     "minItems",
			testExpr: TemplateStr(`len({{ .QualifiedName }}) < ` + minItemsS),
			sprintfExpr: TemplateStr(
				`"must have length greater than ` + minItemsS + ` but was %d", len({{ .QualifiedName }})`,
			),
		})
	}
	if schema.MaxItems != nil {
		maxItemsS := strconv.FormatUint(*schema.MaxItems, 10)
		a.validators = append(a.validators, Validator{
			Name:     "maxItems",
			testExpr: TemplateStr(`len({{ .QualifiedName }}) > ` + maxItemsS),
			sprintfExpr: TemplateStr(
				`"must have length greater than ` + maxItemsS + ` but was %d", len({{ .QualifiedName }})`,
			),
		})
	}
	if schema.UniqueItems {
		if a.ItemType.Name == "interface{}" {
			return nil, errors.New("cannot take unique items of unhashable type")
		}
		a.validators = append(a.validators, Validator{Name: "uniqueItems"})
	}
	if itemSchema != nil {
		a.itemValidators = validators(itemSchema)
	}
	return &a, nil
}

type slicePlanContext struct {
	*generate.Imports
	*SlicePlan
}

func (s *slicePlanContext) Template() string {
	return "slice.tmpl"
}

func (s *SlicePlan) Printable(imp *generate.Imports) generate.PrintablePlan {
	return &slicePlanContext{Imports: imp, SlicePlan: s}
}
