package composite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/validator"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"net/url"
	"sort"
	"unicode"
)

type StructField struct {
	Comment         string
	Name            string
	JSONName        string
	Type            generate.TypeInfo
	Tag             string
	Required        bool
	FieldValidators []validator.Validator
}

func (s StructField) Validators() []validator.Validator {
	return validator.Sorted(s.FieldValidators)
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
			if v.VarExpr != nil {
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

func (s *StructPlan) Execute(imp *generate.Imports) (string, error) {
	var w bytes.Buffer
	err := tmpl.Execute(&w, &structPlanContext{s, imp})
	return w.String(), err
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

//go:generate go run ../cmd/embedtmpl/embedtmpl.go composite struct.tmpl tmpl.gen.go
func PlanObject(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	if schema.ChooseType() != sch.Object {
		return nil, fmt.Errorf("not an object: %w", generate.ErrContinue)
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", generate.ErrContinue)
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
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/pkg/boxed" {
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
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/pkg/boxed", Name: "String", ValPath: "String"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "int64":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/pkg/boxed", Name: "Int64", ValPath: "Int64"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "bool":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/pkg/boxed", Name: "Bool", ValPath: "Bool"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "float64":
				fType = generate.TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/pkg/boxed", Name: "Float64", ValPath: "Float64"}
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
				FieldValidators: validator.Validators(fieldSchema),
			},
		)
	}
	return
}

type structPlanContext struct {
	*StructPlan
	*generate.Imports
}

type EnrichedStructField struct {
	StructField
	StructPlan *StructPlan
	Imports    *generate.Imports
}

func (s *structPlanContext) Fields() (fields []EnrichedStructField) {
	for _, f := range s.StructPlan.Fields {
		fields = append(fields, EnrichedStructField{
			StructField: f,
			StructPlan:  s.StructPlan,
			Imports:     s.Imports,
		})
	}
	return
}

func (f *EnrichedStructField) DerefExpr() string {
	valPath := ""
	if f.Type.ValPath != "" {
		valPath = "." + f.Type.ValPath
	}
	return fmt.Sprintf("m.%s%s", f.Name, valPath)
}

func (f *EnrichedStructField) TestSetExpr(pos bool) (string, error) {
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/pkg/boxed" {
		op := ""
		if !pos {
			op = "!"
		}
		return fmt.Sprintf("%sm.%s.Set", op, f.Name), nil
	}
	if f.Type.Name == "interface{}" || f.Type.Pointer {
		op := "!="
		if !pos {
			op = "=="
		}
		return fmt.Sprintf("m.%s %s nil", f.Name, op), nil
	}
	return "", errors.New("no test set expr")
}

func (f *EnrichedStructField) NameSpace() string {
	name := fmt.Sprintf("%s%s", f.StructPlan.Type().Name, f.Name)
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

func (f *EnrichedStructField) FieldDecl() string {
	typ := f.Imports.QualName(f.Type)
	if f.Type.Pointer {
		typ = "*" + typ
	}
	tag := f.Tag
	if tag != "" {
		tag = "`" + tag + "`"
	}
	return f.Name + " " + typ + tag
}

func (f *EnrichedStructField) InnerFieldDecl() string {
	typName := f.Imports.QualName(f.Type)
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/pkg/boxed" {
		s := []rune(f.Type.Name)
		s[0] = unicode.ToLower(s[0])

		typName = "*" + string(s)
	}
	tag := ""
	if f.Name != "" { // not an embedded struct
		tag = fmt.Sprintf("`json:"+`"%s,omitempty"`+"`", f.JSONName)
	}
	return fmt.Sprintf("%s %s %s", f.Name, typName, tag)
}

func (f *EnrichedStructField) Embedded() bool {
	return f.Name == ""
}

func (f *EnrichedStructField) FieldRef() string {
	if f.Name != "" {
		return f.Name
	}
	return f.Type.Name // embedded
}

func (f *EnrichedStructField) InnerFieldLiteral() string {
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/pkg/boxed" {
		return ""
	}
	fieldRef := f.Name
	if fieldRef == "" { // embedded
		fieldRef = f.Type.Name
	}
	return fmt.Sprintf("%s: m.%s,", fieldRef, fieldRef)
}

var fieldAssignmentTmpl = validator.TemplateStr(`if m.{{ .Name }}.Set {
	inner.{{ .Name }} = &m.{{ .Name }}{{ .ValPath }}
}`)

func (f *EnrichedStructField) InnerFieldAssignment() (string, error) {
	if f.Type.GoPath != "github.com/jwilner/jsonschema2go/pkg/boxed" {
		return "", nil
	}

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
	helper generate.Helper,
	parent *sch.Schema,
	schemas []*sch.RefOrSchema,
) (sch.SimpleType, []*sch.Schema, error) {
	var (
		resolved  []*sch.Schema
		foundType sch.SimpleType
	)
	for _, s := range schemas {
		r, err := s.Resolve(ctx, parent, helper)
		if err != nil {
			return sch.Unknown, nil, err
		}
		resolved = append(resolved, r)
		t := r.ChooseType()
		if t == sch.Unknown {
			continue
		}
		if foundType == sch.Unknown {
			foundType = t
			continue
		}
	}
	return foundType, resolved, nil
}
