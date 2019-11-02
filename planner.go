package jsonschema2go

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type TypeInfo struct {
	GoPath  string
	Name    string
	Pointer bool
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
	Members  []EnumMember
}

type EnumMember struct {
	Name  string
	Field interface{}
}

func (e *EnumPlan) Literal(val interface{}) string {
	switch t := val.(type) {
	case bool:
		return strconv.FormatBool(t)
	case string:
		return fmt.Sprintf("%q", t)
	default:
		return fmt.Sprintf("%d", t)
	}
}

func (e *EnumPlan) Type() TypeInfo {
	return e.typeInfo
}

func (e *EnumPlan) Deps() []TypeInfo {
	return []TypeInfo{e.BaseType}
}

func newPlanner() *Planners {
	return &Planners{
		planners: []Planner{
			plannerFunc(planAllOfObject),
			plannerFunc(planSimpleObject),
			plannerFunc(planSimpleArray),
			plannerFunc(planEnum),
		},
	}
}

type Planners struct {
	planners []Planner
}

type Planner interface {
	Plan(schema *Schema) (Plan, []*Schema)
}

type plannerFunc func(schema *Schema) (Plan, []*Schema)

func (p plannerFunc) Plan(schema *Schema) (Plan, []*Schema) {
	return p(schema)
}

func (p *Planners) Plan(s *Schema) ([]Plan, error) {
	var (
		plans []Plan
		stack = []*Schema{s}
	)

	// dfs
	for len(stack) > 0 {
		schema := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		plan, deps, err := p.derivePlan(schema)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
		stack = append(stack, deps...)
	}
	return plans, nil
}

func (p *Planners) derivePlan(schema *Schema) (Plan, []*Schema, error) {
	for _, p := range p.planners {
		pl, deps := p.Plan(schema)
		if pl == nil {
			continue
		}
		return pl, deps, nil
	}

	return nil, nil, errors.New("unable to plan")
}

func deriveStructFields(schema *Schema) (fields []StructField, deps []*Schema, ok bool) {
	ok = true

	var properties []string
	for k := range schema.Properties {
		properties = append(properties, k)
	}
	sort.Strings(properties)

	for _, name := range properties {
		fieldSchema := schema.Properties[name]
		fType, ok := deriveTypeInfo(fieldSchema)
		if !ok &&
			len(fieldSchema.OneOf) == 2 &&
			(fieldSchema.OneOf[0].ChooseType() == Null || fieldSchema.OneOf[1].ChooseType() == Null) {
			// this is a nillable field
			valueSchema := fieldSchema.OneOf[0]
			if valueSchema.ChooseType() == Null {
				valueSchema = fieldSchema.OneOf[1]
			}
			fieldSchema = valueSchema

			if fType, ok = deriveTypeInfo(fieldSchema); !ok {
				return nil, nil, false
			}
			fType.Pointer = true
		} else if !ok {
			return nil, nil, false
		}
		fields = append(
			fields,
			StructField{
				Comment: fieldSchema.Annotations.GetString("description"),
				Names:   []string{jsonPropertyToExportedName(name)},
				Type:    fType,
				Tag:     fmt.Sprintf(`json:"%s,omitempty"`, name),
			},
		)
		if !fType.BuiltIn() {
			deps = append(deps, fieldSchema)
		}
	}
	return
}

func deriveTypeInfo(s *Schema) (TypeInfo, bool) {
	switch s.ChooseType() {
	case Boolean:
		return BuiltInBool, true
	case Integer:
		return BuiltInInt, true
	case Null:
		return BuiltInNull, true
	case Number:
		return BuiltInFloat, true
	case String:
		return BuiltInString, true
	}
	return getGoPathInfo(s)
}

func getGoPathInfo(s *Schema) (TypeInfo, bool) {
	parts := strings.SplitN(s.Annotations.GetString("x-gopath"), "#", 2)
	if len(parts) != 2 {
		return TypeInfo{}, false
	}
	return TypeInfo{GoPath: parts[0], Name: parts[1]}, true
}

func planAllOfObject(schema *Schema) (_ Plan, deps []*Schema) {
	if len(schema.AllOf) == 0 || schema.AllOf[0].ChooseType() != Object {
		return nil, nil
	}
	tInfo, ok := deriveTypeInfo(schema)
	if !ok {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")

	for _, subSchema := range schema.AllOf {
		tInfo, ok := deriveTypeInfo(subSchema)
		if !ok {
			// this is an anonymous struct; add all of its inner fields to parent
			fields, infos, ok := deriveStructFields(subSchema)
			if !ok {
				return nil, nil
			}
			s.Fields = append(s.Fields, fields...)
			deps = append(deps, infos...)
			continue
		}
		// this is a named type, add an embedded field for the subschema type
		s.Fields = append(s.Fields, StructField{Type: tInfo})
		deps = append(deps, subSchema)
	}
	return s, deps
}

func planSimpleObject(schema *Schema) (_ Plan, deps []*Schema) {
	if schema.ChooseType() != Object {
		return nil, nil
	}
	tInfo, ok := deriveTypeInfo(schema)
	if !ok {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")
	fields, infos, ok := deriveStructFields(schema)
	if !ok {
		return nil, nil
	}
	s.Fields = fields
	deps = append(deps, infos...)
	return s, deps
}

func planSimpleArray(schema *Schema) (_ Plan, deps []*Schema) {
	if schema.ChooseType() != Array {
		return nil, nil
	}
	tInfo, ok := deriveTypeInfo(schema)
	if !ok {
		return nil, nil
	}
	if schema.Items != nil && schema.Items.Items != nil {
		a := &ArrayPlan{typeInfo: tInfo}
		a.Comment = schema.Annotations.GetString("description")
		if a.ItemType, ok = deriveTypeInfo(schema.Items.Items); !ok {
			return nil, nil
		}
		if !a.ItemType.BuiltIn() {
			deps = append(deps, schema.Items.Items)
		}
		return a, deps
	}
	return nil, nil
}

func planEnum(schema *Schema) (_ Plan, _ []*Schema) {
	if len(schema.Enum) == 0 {
		return nil, nil
	}

	tInfo, ok := getGoPathInfo(schema)
	if !ok {
		return nil, nil
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
	for _, m := range schema.Enum {
		name := jsonPropertyToExportedName(fmt.Sprintf("%s", m))
		e.Members = append(e.Members, EnumMember{Name: name, Field: m})
	}
	return e, nil
}

var knownInitialisms = map[string]bool{
	"id":   true,
	"http": true,
}

func jsonPropertyToExportedName(name string) string {
	if strings.ToUpper(name) == name {
		return exportedIdentifier([][]rune{[]rune(strings.ToLower(name))})
	}

	var (
		current []rune
		parts   [][]rune
	)
	// split words
	for _, r := range []rune(name) {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			// exclusive word boundary
			if len(current) != 0 {
				parts = append(parts, current)
				current = nil
			}
			continue
		}
		if unicode.IsUpper(r) {
			// inclusive word boundary
			if len(current) != 0 {
				parts = append(parts, current)
			}
			current = []rune{unicode.ToLower(r)}
			continue
		}

		current = append(current, r)
	}

	if len(current) > 0 {
		parts = append(parts, current)
	}

	return exportedIdentifier(parts)
}

func exportedIdentifier(parts [][]rune) string {
	var words []string
	for _, rs := range parts {
		if word := string(rs); knownInitialisms[word] {
			words = append(words, strings.ToUpper(word))
			continue
		}
		rs[0] = unicode.ToUpper(rs[0])
		words = append(words, string(rs))
	}
	return strings.Join(words, "")
}
