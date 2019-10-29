package jsonschema2go

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

type TypeInfo struct {
	GoPath   string
	Name     string
	FileName string
	Pointer  bool
}

func (t TypeInfo) Package() string {
	if t.GoPath == "" {
		return ""
	}
	return path.Base(t.GoPath)
}

func (t TypeInfo) BuiltIn() bool {
	return t.GoPath == ""
}

var (
	BuiltInBool          = TypeInfo{Name: "bool"}
	BuiltInBoolPointer   = TypeInfo{Name: "bool", Pointer: true}
	BuiltInInt           = TypeInfo{Name: "int"}
	BuiltInIntPointer    = TypeInfo{Name: "int", Pointer: true}
	BuiltInFloat         = TypeInfo{Name: "float64"}
	BuiltInFloatPointer  = TypeInfo{Name: "float64", Pointer: true}
	BuiltInNull          = TypeInfo{Name: "null"}
	BuiltInString        = TypeInfo{Name: "string"}
	BuiltInStringPointer = TypeInfo{Name: "string", Pointer: true}

	primitives = map[SimpleType]TypeInfo{
		Boolean: BuiltInBool,
		Integer: BuiltInInt,
		Number:  BuiltInFloat,
		Null:    BuiltInNull,
		String:  BuiltInString,
	}
)

type StructField struct {
	Comment string
	Names   []string
	Type    TypeInfo
	Tag     string
}

type Plan interface {
	Type() TypeInfo
	Deps() []TypeInfo
}

type StructPlan struct {
	typeInfo TypeInfo

	Comment string
	Fields  []StructField
}

func (s *StructPlan) Type() TypeInfo {
	return s.typeInfo
}

func (s *StructPlan) Deps() (deps []TypeInfo) {
	for _, s := range s.Fields {
		deps = append(deps, s.Type)
	}
	return
}

type ArrayPlan struct {
	typeInfo TypeInfo

	Comment  string
	ItemType TypeInfo
}

func (a *ArrayPlan) Type() TypeInfo {
	return a.typeInfo
}

func (a *ArrayPlan) Deps() []TypeInfo {
	return []TypeInfo{a.ItemType, {Name: "Marshal", GoPath: "encoding/json"}}
}

type EnumPlan struct {
	typeInfo TypeInfo

	Comment  string
	BaseType TypeInfo
	Members  []interface{}
}

func (e *EnumPlan) Type() TypeInfo {
	return e.typeInfo
}

func (e *EnumPlan) Deps() []TypeInfo {
	return []TypeInfo{e.BaseType}
}

type SchemeTypeInfo struct {
	Schema   *Schema
	TypeInfo TypeInfo
}

func SchemaToPlan(s *Schema) ([]Plan, error) {
	var (
		plans           []Plan
		schemaTypeInfos []SchemeTypeInfo
	)

	// init stack
	{
		tInfo, err := deriveTypeInfo(s)
		if err != nil {
			return nil, err
		}

		schemaTypeInfos = append(schemaTypeInfos, SchemeTypeInfo{s, tInfo})
	}

	// dfs
	for len(schemaTypeInfos) > 0 {
		schemaTypeInfo := schemaTypeInfos[len(schemaTypeInfos)-1]
		schemaTypeInfos = schemaTypeInfos[:len(schemaTypeInfos)-1]

		schema := schemaTypeInfo.Schema
		tInfo := schemaTypeInfo.TypeInfo

		valPlan, deps, err := derivePlan(tInfo, schema)
		if err != nil {
			return nil, err
		}
		plans = append(plans, valPlan)
		schemaTypeInfos = append(schemaTypeInfos, deps...)
	}
	return plans, nil
}

var errPlanContinue = errors.New("continue planning")

func derivePlan(tInfo TypeInfo, schema *Schema) (Plan, []SchemeTypeInfo, error) {
	planners := []func(tInfo TypeInfo, schema *Schema) (Plan, []SchemeTypeInfo, error){
		planAllOfObject,
		planSimpleObject,
		planSimpleArray,
		planEnum,
	}

	for len(planners) > 0 {
		p, deps, err := planners[0](tInfo, schema)
		if errors.Is(err, errPlanContinue) {
			planners = planners[1:]
			continue
		}
		return p, deps, err
	}

	return nil, nil, errors.New("unable to plan")
}

func deriveStructFields(schema *Schema) (fields []StructField, infos []SchemeTypeInfo, _ error) {
	var properties []string
	for k := range schema.Properties {
		properties = append(properties, k)
	}
	sort.Strings(properties)

	for _, name := range properties {
		fieldSchema := schema.Properties[name]
		fType, err := deriveTypeInfo(fieldSchema)
		if errors.Is(err, errAnonymous) &&
			len(fieldSchema.OneOf) == 2 &&
			(fieldSchema.OneOf[0].ChooseType() == Null || fieldSchema.OneOf[1].ChooseType() == Null) {
			// this is a nillable field
			valueSchema := fieldSchema.OneOf[0]
			if valueSchema.ChooseType() == Null {
				valueSchema = fieldSchema.OneOf[1]
			}
			fieldSchema = valueSchema

			if fType, err = deriveTypeInfo(fieldSchema); err != nil {
				return nil, nil, err
			}
			fType.Pointer = true
		} else if err != nil {
			return nil, nil, err
		}
		fields = append(
			fields,
			StructField{
				Comment: fieldSchema.Annotations.GetString("description"),
				Names:   []string{exportedCamelCase(name)},
				Type:    fType,
				Tag:     fmt.Sprintf(`json="%s,omitempty"`, name),
			},
		)
		if !fType.BuiltIn() {
			infos = append(infos, SchemeTypeInfo{fieldSchema, fType})
		}
	}
	return
}

var errAnonymous = errors.New("anonymous type")

func deriveTypeInfo(s *Schema) (TypeInfo, error) {
	switch s.ChooseType() {
	case Boolean:
		return BuiltInBool, nil
	case Integer:
		return BuiltInInt, nil
	case Null:
		return BuiltInNull, nil
	case Number:
		return BuiltInFloat, nil
	}

	var l TypeInfo
	{
		parts := strings.SplitN(s.Annotations.GetString("x-go-import-path"), "#", 2)
		switch len(parts) {
		case 2:
			l.Name = parts[1]
			fallthrough
		case 1:
			l.GoPath = parts[0]
		}
	}

	if l.GoPath == "" {
		return l, fmt.Errorf("no path: %w", errAnonymous)
	}
	if l.Name == "" {
		return l, fmt.Errorf("no name: %w", errAnonymous)
	}
	l.FileName = "values.gen.go"

	return l, nil
}

func exportedCamelCase(s string) string {
	return s
}

func planAllOfObject(tInfo TypeInfo, schema *Schema) (_ Plan, deps []SchemeTypeInfo, _ error) {
	if len(schema.AllOf) == 0 || schema.AllOf[0].ChooseType() != Object {
		return nil, nil, errPlanContinue
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")

	for _, subSchema := range schema.AllOf {
		tInfo, err := deriveTypeInfo(subSchema)
		if errors.Is(err, errAnonymous) {
			// this is an anonymous struct; add all of its inner fields to parent
			fields, infos, err := deriveStructFields(subSchema)
			if err != nil {
				return nil, nil, err
			}
			s.Fields = append(s.Fields, fields...)
			deps = append(deps, infos...)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		// this is a named type, add an embedded field for the subschema type
		s.Fields = append(s.Fields, StructField{Type: tInfo})
		deps = append(deps, SchemeTypeInfo{subSchema, tInfo})
	}
	return s, deps, nil
}

func planSimpleObject(tInfo TypeInfo, schema *Schema) (_ Plan, deps []SchemeTypeInfo, _ error) {
	if schema.ChooseType() != Object {
		return nil, nil, errPlanContinue
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")
	fields, infos, err := deriveStructFields(schema)
	if err != nil {
		return nil, nil, err
	}
	s.Fields = fields
	deps = append(deps, infos...)
	return s, deps, nil
}

func planSimpleArray(tInfo TypeInfo, schema *Schema) (_ Plan, deps []SchemeTypeInfo, _ error) {
	if schema.ChooseType() != Array {
		return nil, nil, errPlanContinue
	}
	if schema.Items != nil && schema.Items.Items != nil {
		a := &ArrayPlan{typeInfo: tInfo}
		a.Comment = schema.Annotations.GetString("description")
		var err error
		if a.ItemType, err = deriveTypeInfo(schema.Items.Items); err != nil {
			return nil, nil, err
		}
		if !a.ItemType.BuiltIn() {
			deps = append(deps, SchemeTypeInfo{schema.Items.Items, a.ItemType})
		}
		return a, deps, nil
	}
	return nil, nil, errors.New("don't support this type of array")
}

func planEnum(tInfo TypeInfo, schema *Schema) (_ Plan, deps []SchemeTypeInfo, _ error) {
	if len(schema.Enum) == 0 {
		return nil, nil, errPlanContinue
	}
	e := &EnumPlan{typeInfo: tInfo}
	e.Comment = schema.Annotations.GetString("description")
	switch schema.ChooseType() {
	case String:
		e.BaseType = BuiltInString
	case Integer:
		e.BaseType = BuiltInInt
	case Number:
		e.BaseType = BuiltInFloat
	}
	e.Members = append(e.Members, schema.Enum...)
	return e, nil, nil
}
