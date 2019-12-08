package slice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/templates"
	"github.com/jwilner/jsonschema2go/internal/validator"
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
	validators     []validator.Validator
	itemValidators []validator.Validator
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

func (a *SlicePlan) Validators() []validator.Validator {
	sort.Slice(a.validators, func(i, j int) bool {
		return a.validators[i].Name < a.validators[j].Name
	})
	return a.validators
}

func (a *SlicePlan) ItemValidators() []validator.Validator {
	sort.Slice(a.itemValidators, func(i, j int) bool {
		return a.itemValidators[i].Name < a.itemValidators[j].Name
	})
	return a.itemValidators
}

func (a *SlicePlan) ItemValidateInitialize() bool {
	for _, i := range a.itemValidators {
		if i.VarExpr != nil {
			return true
		}
	}
	return false
}

//go:generate go run ../cmd/embedtmpl/embedtmpl.go slice slice.tmpl tmpl.gen.go
func PlanSlice(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	if schema.ChooseType() != sch.Array {
		return nil, fmt.Errorf("not an array: %w", generate.ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", generate.ErrContinue)
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
		a.validators = append(a.validators, validator.Validator{
			Name:     "minItems",
			TestExpr: validator.TemplateStr(`len({{ .QualifiedName }}) < ` + minItemsS),
			SprintfExpr: validator.TemplateStr(
				`"must have length greater than ` + minItemsS + ` but was %d", len({{ .QualifiedName }})`,
			),
		})
	}
	if schema.MaxItems != nil {
		maxItemsS := strconv.FormatUint(*schema.MaxItems, 10)
		a.validators = append(a.validators, validator.Validator{
			Name:     "maxItems",
			TestExpr: validator.TemplateStr(`len({{ .QualifiedName }}) > ` + maxItemsS),
			SprintfExpr: validator.TemplateStr(
				`"must have length greater than ` + maxItemsS + ` but was %d", len({{ .QualifiedName }})`,
			),
		})
	}
	if schema.UniqueItems {
		if a.ItemType.Name == "interface{}" {
			return nil, errors.New("cannot take unique items of unhashable type")
		}
		a.validators = append(a.validators, validator.Validator{Name: "uniqueItems"})
	}
	if itemSchema != nil {
		a.itemValidators = validator.Validators(itemSchema)
	}
	return &a, nil
}

type slicePlanContext struct {
	*generate.Imports
	*SlicePlan
}

func (s *SlicePlan) Execute(imp *generate.Imports) (string, error) {
	var w bytes.Buffer
	err := templates.Template.ExecuteTemplate(&w, "slice.tmpl", slicePlanContext{imp, s})
	return w.String(), err
}
