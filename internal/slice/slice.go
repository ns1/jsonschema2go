package slice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/validator"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"net/url"
	"sort"
	"strconv"
)

//go:generatego run ../cmd/embedtmpl/embedtmpl.go slice slice.tmpl tmpl.gen.go

// Build attempts to generate the plan for a slice from the provided schema
func Build(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	if schema.ChooseType() != gen.Array {
		return nil, fmt.Errorf("not an array: %w", gen.ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", gen.ErrContinue)
	}

	// we've matched
	var itemSchema *gen.Schema
	if schema.Items != nil && schema.Items.Items != nil {
		var err error
		if itemSchema, err = schema.Items.Items.Resolve(ctx, schema, helper); err != nil {
			return nil, err
		}
	}
	a := Plan{TypeInfo: tInfo, ID: schema.CalcID}
	a.Comment = schema.Annotations.GetString("description")
	if itemSchema != nil {
		if a.ItemType = helper.TypeInfo(itemSchema); a.ItemType.Unknown() {
			return nil, nil
		}
	} else {
		a.ItemType = gen.TypeInfo{Name: "interface{}"}
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

// Plan encapsulates information for rendering the source code of a slice
type Plan struct {
	TypeInfo gen.TypeInfo
	ID       *url.URL

	Comment        string
	ItemType       gen.TypeInfo
	validators     []validator.Validator
	itemValidators []validator.Validator
}

// Type returns the TypeInfo for this plan
func (p *Plan) Type() gen.TypeInfo {
	return p.TypeInfo
}

// Deps returns any known dependencies of this plan
func (p *Plan) Deps() []gen.TypeInfo {
	return []gen.TypeInfo{p.ItemType, {Name: "Marshal", GoPath: "encoding/json"}, {Name: "Sprintf", GoPath: "fmt"}}
}

// Validators returns validators of the slice itself
func (p *Plan) Validators() []validator.Validator {
	sort.Slice(p.validators, func(i, j int) bool {
		return p.validators[i].Name < p.validators[j].Name
	})
	return p.validators
}

// ItemValidators returns validators of the items within the slice itself
func (p *Plan) ItemValidators() []validator.Validator {
	sort.Slice(p.itemValidators, func(i, j int) bool {
		return p.itemValidators[i].Name < p.itemValidators[j].Name
	})
	return p.itemValidators
}

// ItemValidateInitialize returns whether there are any item validators which require initialization
func (p *Plan) ItemValidateInitialize() bool {
	for _, i := range p.itemValidators {
		if i.VarExpr != nil {
			return true
		}
	}
	return false
}

// Execute renders the current plan as a string
func (p *Plan) Execute(imp *gen.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, slicePlanContext{imp, p})
	return w.String(), err
}

type slicePlanContext struct {
	*gen.Imports
	*Plan
}
