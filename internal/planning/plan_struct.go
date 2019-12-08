package planning

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"net/url"
	"sort"
)

type StructField struct {
	Comment         string
	Name            string
	JSONName        string
	Type            generate.TypeInfo
	Tag             string
	Required        bool
	FieldValidators []Validator
}

func (s StructField) Validators() []Validator {
	return sortedValidators(s.FieldValidators)
}

type StructPlan struct {
	TypeInfo generate.TypeInfo
	Id       *url.URL

	Comment string
	Fields  []StructField
	Traits  []Trait
}

func (s *StructPlan) Type() generate.TypeInfo {
	return s.TypeInfo
}

func (s *StructPlan) ValidateInitialize() bool {
	for _, f := range s.Fields {
		for _, v := range f.FieldValidators {
			if v.varExpr != nil {
				return true
			}
		}
	}
	return false
}

func (s *StructPlan) ID() string {
	if s.Id != nil {
		return s.Id.String()
	}
	return ""
}

func (s *StructPlan) Deps() (deps []generate.TypeInfo) {
	deps = append(deps, generate.TypeInfo{Name: "Sprintf", GoPath: "fmt"})
	for _, f := range s.Fields {
		deps = append(deps, f.Type)
		for _, v := range f.FieldValidators {
			deps = append(deps, v.Deps...)
		}
	}
	for _, t := range s.Traits {
		if t, ok := t.(interface{ Deps() []generate.TypeInfo }); ok {
			deps = append(deps, t.Deps()...)
		}
	}
	return
}

func planObject(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	if schema.ChooseType() != sch.Object {
		return nil, fmt.Errorf("not an object: %w", ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", ErrContinue)
	}
	// matched

	s := &StructPlan{TypeInfo: tInfo, Id: schema.CalcID}
	s.Comment = schema.Annotations.GetString("description")
	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields

	for _, f := range fields {
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
			s.Traits = append(s.Traits, &BoxedEncodingTrait{})
			break
		}
	}

	return s, nil
}

func deriveStructFields(
	ctx context.Context,
	helper generate.Helper,
	schema *sch.Schema,
) (fields []StructField, _ error) {
	required := make(map[string]bool, len(schema.Required))
	for _, k := range schema.Required {
		required[k] = true
	}

	var properties []string
	for k := range schema.Properties {
		properties = append(properties, k)
	}
	sort.Strings(properties)

	for _, name := range properties {
		fieldSchema, err := schema.Properties[name].Resolve(ctx, schema, helper)
		if err != nil {
			return nil, err
		}
		fType := helper.TypeInfo(fieldSchema)
		if fType.Unknown() && len(fieldSchema.OneOf) == 2 {
			oneOfA, err := fieldSchema.OneOf[0].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			oneOfB, err := fieldSchema.OneOf[1].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			if oneOfA.ChooseType() == sch.Null || oneOfB.ChooseType() == sch.Null {
				// this is a nillable field
				valueSchema := oneOfA
				if valueSchema.ChooseType() == sch.Null {
					valueSchema = oneOfB
				}
				if fType = helper.TypeInfo(valueSchema); fType.Unknown() {
					return nil, nil
				}
				fType.Pointer = true
			}
		}
		if fieldSchema.ChooseType() == sch.Unknown && fType.Unknown() {
			fType = generate.TypeInfo{Name: "interface{}"}
		}
		if !fType.BuiltIn() {
			if err := helper.Dep(ctx, fieldSchema); err != nil {
				return nil, err
			}
		}
		tag := fmt.Sprintf(`json:"%s,omitempty"`, name)
		if fType.BuiltIn() && !fType.Pointer {
			switch fType.Name {
			case "string":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "String", ValPath: "String"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "int64":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Int64", ValPath: "Int64"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "bool":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Bool", ValPath: "Bool"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "float64":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Float64", ValPath: "Float64"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			}
		}
		fields = append(
			fields,
			StructField{
				Comment:         fieldSchema.Annotations.GetString("description"),
				Name:            helper.JSONPropertyExported(name),
				JSONName:        name,
				Type:            fType,
				Tag:             tag,
				Required:        required[name],
				FieldValidators: validators(fieldSchema),
			},
		)
	}
	return
}
