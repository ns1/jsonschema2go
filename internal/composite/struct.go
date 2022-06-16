package composite

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sort"
	"unicode"

	"github.com/ns1/jsonschema2go/internal/validator"
	"github.com/ns1/jsonschema2go/pkg/gen"
)

// StructField contains information about how a struct's field should be rendered
type StructField struct {
	Comment         string
	Name            string
	JSONName        string
	Type            gen.TypeInfo
	Tag             string
	Required        bool
	FieldValidators []validator.Validator
}

// Validators returns the validators for this field
func (s StructField) Validators() []validator.Validator {
	return validator.Sorted(s.FieldValidators)
}

// StructPlan is an implementation of the interface Plan specific to structs
type StructPlan struct {
	TypeInfo gen.TypeInfo
	ID       *url.URL

	Comment     string
	Fields      []StructField
	SubRequired []StructField
	Traits      []Trait
}

// Type returns the calculated type info for this struct
func (s *StructPlan) Type() gen.TypeInfo {
	return s.TypeInfo
}

// Execute executes the provided struct plan and returns it rendered as a string
func (s *StructPlan) Execute(imp *gen.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &structPlanContext{s, imp})
	return w.String(), err
}

// Deps returns all known required imported symbols for this plan
func (s *StructPlan) Deps() (deps []gen.TypeInfo) {
	deps = append(deps, gen.TypeInfo{Name: "Sprintf", GoPath: "fmt"})
	for _, f := range s.Fields {
		deps = append(deps, f.Type)
		for _, v := range f.FieldValidators {
			deps = append(deps, v.Deps...)
		}
	}
	for _, t := range s.Traits {
		if t, ok := t.(interface{ Deps() []gen.TypeInfo }); ok {
			deps = append(deps, t.Deps()...)
		}
	}
	return
}

//go:generate go run ../cmd/embedtmpl/embedtmpl.go composite struct.tmpl tmpl.gen.go

// PlanObject returns a plan if the provided type is an object; otherwise it returns ErrContinue
func PlanObject(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	jType, err := helper.DetectSimpleType(ctx, schema)
	if err != nil {
		return nil, err
	}
	if jType != gen.JSONObject {
		return nil, fmt.Errorf("not an object: %w", gen.ErrContinue)
	}
	tInfo, err := helper.TypeInfo(schema)
	if err != nil {
		return nil, err
	}
	// matched

	s := &StructPlan{TypeInfo: tInfo, ID: schema.ID}
	s.Comment = schema.Annotations.GetString("description")
	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields

	return s, nil
}

func deriveStructFields(
	ctx context.Context,
	helper gen.Helper,
	schema *gen.Schema,
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

		if fieldSchema.Config.RawMessage {
			fields = append(
				fields,
				StructField{
					Comment:  fieldSchema.Annotations.GetString("description"),
					Name:     helper.JSONPropertyExported(name),
					JSONName: name,
					Type:     gen.TypeInfo{GoPath: "encoding/json", Name: "RawMessage"},
					Tag: func() string {
						omitEmpty := ""
						if !fieldSchema.Config.NoOmitEmpty {
							omitEmpty = ",omitempty"
						}
						return fmt.Sprintf("`"+`json:"%s%s"`+"`", name, omitEmpty)
					}(),
					Required:        required[name],
					FieldValidators: validator.Validators(fieldSchema),
				},
			)
			continue
		}

		fType, _ := helper.TypeInfo(fieldSchema)
		if fType.Unknown() && len(fieldSchema.OneOf) == 2 {
			oneOfA, err := fieldSchema.OneOf[0].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			typeA, err := helper.DetectSimpleType(ctx, oneOfA)
			if err != nil {
				return nil, err
			}
			oneOfB, err := fieldSchema.OneOf[1].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			typeB, err := helper.DetectSimpleType(ctx, oneOfB)
			if err != nil {
				return nil, err
			}
			if typeA == gen.JSONNull || typeB == gen.JSONNull {
				// this is a nillable field
				valueSchema := oneOfA
				if typeA == gen.JSONNull {
					valueSchema = oneOfB
				}
				if fType, err = helper.TypeInfo(valueSchema); err != nil {
					return nil, err
				}
				fType.Pointer = true
			}
		}
		fJType, err := helper.DetectSimpleType(ctx, fieldSchema)
		if err != nil && !helper.ErrSimpleTypeUnknown(err) {
			return nil, err
		}
		if fJType == gen.JSONObject &&
			len(fieldSchema.Properties) == 0 &&
			fieldSchema.AdditionalProperties != nil &&
			fieldSchema.AdditionalProperties.Bool != nil &&
			*fieldSchema.AdditionalProperties.Bool {
			fType = gen.TypeInfo{Name: "map[string]interface{}"}
		}
		if fJType == gen.JSONUnknown && fType.Unknown() {
			fType = gen.TypeInfo{Name: "interface{}"}
		}
		if !fType.BuiltIn() {
			if err := helper.Dep(ctx, fieldSchema); err != nil {
				return nil, err
			}
		}

		var tag string
		switch {
		case name == "": // embedded fields don't get tags
		case (fJType == gen.JSONArray && !fieldSchema.Config.OmitEmptyArray) || fieldSchema.Config.NoOmitEmpty:
			tag = fmt.Sprintf("`"+`json:"%s"`+"`", name)
		default:
			tag = fmt.Sprintf("`"+`json:"%s,omitempty"`+"`", name)
		}

		if fType.BuiltIn() {
			switch fType.Name {
			case "string", "int64", "bool", "float64":
				fType.Pointer = true
			}
		}
		// not a reference type
		if !fType.BuiltIn() && fJType == gen.JSONObject && !fieldSchema.AdditionalProperties.Present() {
			fType.Pointer = true
		}

		fieldName, ok := schema.Config.FieldAliases[name]
		if !ok {
			fieldName = helper.JSONPropertyExported(name)
		}

		fields = append(
			fields,
			StructField{
				Comment:         fieldSchema.Annotations.GetString("description"),
				Name:            fieldName,
				JSONName:        name,
				Type:            fType,
				Tag:             tag,
				Required:        required[name],
				FieldValidators: validator.Validators(fieldSchema),
			},
		)
	}
	return
}

type structPlanContext struct {
	*StructPlan
	*gen.Imports
}

func (s *structPlanContext) Comment() string {
	return gen.NormalizeComment(s.StructPlan.Comment)
}

func (s *structPlanContext) ValidateInitialize() bool {
	for _, f := range s.Fields() {
		for _, v := range f.FieldValidators {
			if v.VarExpr != nil {
				return true
			}
		}
	}
	return false
}

type enrichedStructField struct {
	StructField
	StructPlan *StructPlan
	Imports    *gen.Imports
}

func (f *enrichedStructField) Comment() string {
	return gen.NormalizeComment(f.StructField.Comment)
}

func (s *structPlanContext) Required() []enrichedStructField {
	var fields []StructField
	for _, s := range s.StructPlan.Fields {
		if s.Required {
			fields = append(fields, s)
		}
	}
	fields = append(fields, s.SubRequired...)

	return s.enrich(fields)
}

func (s *structPlanContext) enrich(fields []StructField) (enriched []enrichedStructField) {
	for _, f := range fields {
		enriched = append(enriched, enrichedStructField{
			StructField: f,
			StructPlan:  s.StructPlan,
			Imports:     s.Imports,
		})
	}
	return
}

func (s *structPlanContext) Fields() []enrichedStructField {
	return s.enrich(s.StructPlan.Fields)
}

func (f *enrichedStructField) DerefExpr() string {
	valPath := ""
	if f.Type.ValPath != "" {
		valPath = "." + f.Type.ValPath
	}
	v := fmt.Sprintf("m.%s%s", f.Name, valPath)
	if f.Type.Pointer {
		v = "*" + v
	}
	return v
}

func (f *enrichedStructField) TestSetExpr(pos bool) (string, error) {
	op := "!="
	if !pos {
		op = "=="
	}
	return fmt.Sprintf("m.%s %s nil", f.Name, op), nil
}

func (f *enrichedStructField) NameSpace() string {
	name := fmt.Sprintf("%s%s", f.StructPlan.Type().Name, f.Name)
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

func (f *enrichedStructField) FieldDecl() string {
	typ := f.Imports.QualName(f.Type)
	if f.Type.Pointer {
		typ = "*" + typ
	}
	return f.Name + " " + typ + " " + f.Tag
}

func (f *enrichedStructField) InnerFieldDecl() string {
	typName := f.Imports.QualName(f.Type)
	return fmt.Sprintf("%s %s %s", f.Name, typName, f.Tag)
}

func (f *enrichedStructField) Embedded() bool {
	return f.Name == ""
}

func (f *enrichedStructField) FieldRef() string {
	if f.Name != "" {
		return f.Name
	}
	return f.Type.Name // embedded
}

func (f *enrichedStructField) InnerFieldLiteral() string {
	fieldRef := f.Name
	if fieldRef == "" { // embedded
		fieldRef = f.Type.Name
	}
	return fmt.Sprintf("%s: m.%s,", fieldRef, fieldRef)
}

var fieldAssignmentTmpl = validator.TemplateStr(`if m.{{ .Name }}.Set {
	inner.{{ .Name }} = &m.{{ .Name }}{{ .ValPath }}
}`)

func (f *enrichedStructField) InnerFieldAssignment() (string, error) {
	valPath := ""
	if f.Type.ValPath != "" {
		valPath = "." + f.Type.ValPath
	}

	var w bytes.Buffer
	err := fieldAssignmentTmpl.Execute(&w, struct {
		Name    string
		ValPath string
	}{
		Name:    f.Name,
		ValPath: valPath,
	})
	return w.String(), err
}

func loadSchemaList(
	ctx context.Context,
	helper gen.Helper,
	parent *gen.Schema,
	schemas []*gen.RefOrSchema,
) (gen.JSONType, []*gen.Schema, error) {
	var (
		resolved  []*gen.Schema
		foundType gen.JSONType
	)
	for _, s := range schemas {
		r, err := s.Resolve(ctx, parent, helper)
		if err != nil {
			return gen.JSONUnknown, nil, err
		}
		resolved = append(resolved, r)
		t, err := helper.DetectSimpleType(ctx, r)
		if err != nil {
			return gen.JSONUnknown, nil, err
		}
		if t == gen.JSONUnknown {
			continue
		}
		if foundType == gen.JSONUnknown {
			foundType = t
			continue
		}
	}
	return foundType, resolved, nil
}
