package jsonschema2go

import (
	"context"
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

func (t TypeInfo) Unknown() bool {
	return t == TypeInfo{}
}

var primitives = map[SimpleType]string{
	Boolean: "bool",
	Integer: "int",
	Number:  "float",
	Null:    "interface{}",
	String:  "string",
}

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
	Plan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, []*Schema)
}

type plannerFunc func(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, []*Schema)

func (p plannerFunc) Plan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, []*Schema) {
	return p(ctx, helper, schema)
}

func (p *Planners) Plan(ctx context.Context, s *Schema, loader Loader) ([]Plan, error) {
	var (
		plans  []Plan
		stack  = []*Schema{s}
		helper = &PlanningHelper{loader}
	)

	for len(stack) > 0 {
		schema := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		plan, deps, err := p.derivePlan(ctx, helper, schema)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
		stack = append(stack, deps...)
	}
	return plans, nil
}

func (p *Planners) derivePlan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, []*Schema, error) {
	for _, p := range p.planners {
		pl, deps := p.Plan(ctx, helper, schema)
		if pl == nil {
			continue
		}
		return pl, deps, nil
	}

	return nil, nil, errors.New("unable to plan")
}

func deriveStructFields(
	ctx context.Context,
	helper *PlanningHelper,
	schema *Schema,
) (fields []StructField, deps []*Schema, ok bool) {
	ok = true

	var properties []string
	for k := range schema.Properties {
		properties = append(properties, k)
	}
	sort.Strings(properties)

	for _, name := range properties {
		fieldSchema, err := schema.Properties[name].Resolve(ctx, schema, helper)
		if err != nil {
			return nil, nil, false
		}

		fType := helper.TypeInfo(fieldSchema.Meta())
		if fType.Unknown() && len(fieldSchema.OneOf) == 2 {
			oneOfA, err := fieldSchema.OneOf[0].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, nil, false
			}
			oneOfB, err := fieldSchema.OneOf[1].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, nil, false
			}
			if oneOfA.ChooseType() == Null || oneOfB.ChooseType() == Null {
				// this is a nillable field
				valueSchema := oneOfA
				if valueSchema.ChooseType() == Null {
					valueSchema = oneOfB
				}
				if fType = helper.TypeInfo(valueSchema.Meta()); fType.Unknown() {
					return nil, nil, false
				}
				fType.Pointer = true
			}
		}
		if fType.Unknown() {
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

type PlanningHelper struct {
	Loader
}

func (p *PlanningHelper) TypeInfo(s SchemaMeta) TypeInfo {
	parts := strings.SplitN(s.Annotations.GetString("x-gopath"), "#", 2)
	if len(parts) == 2 {
		return TypeInfo{GoPath: parts[0], Name: parts[1]}
	}
	return TypeInfo{Name: p.Primitive(s.BestType)}
}

func (p *PlanningHelper) Primitive(s SimpleType) string {
	return primitives[s]
}

func planAllOfObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (_ Plan, deps []*Schema) {
	if len(schema.AllOf) == 0 {
		return nil, nil
	}
	var (
		resolved []*Schema
		foundObj bool
	)

	for _, s := range schema.AllOf {
		r, err := s.Resolve(ctx, schema, helper)
		if err != nil {
			return nil, nil
		}
		resolved = append(resolved, r)
		if !foundObj && r.ChooseType() == Object {
			foundObj = true
		}
	}
	if !foundObj {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")

	for _, subSchema := range resolved {
		tInfo := helper.TypeInfo(subSchema.Meta())
		if tInfo.Unknown() {
			// this is an anonymous struct; add all of its inner fields to parent
			fields, infos, ok := deriveStructFields(ctx, helper, subSchema)
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

func planSimpleObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (_ Plan, deps []*Schema) {
	if schema.ChooseType() != Object {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")
	fields, infos, ok := deriveStructFields(ctx, helper, schema)
	if !ok {
		return nil, nil
	}
	s.Fields = fields
	deps = append(deps, infos...)
	return s, deps
}

func planSimpleArray(ctx context.Context, helper *PlanningHelper, schema *Schema) (_ Plan, deps []*Schema) {
	if schema.ChooseType() != Array || schema.Items == nil || schema.Items.Items == nil {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}

	itemSchema, err := schema.Items.Items.Resolve(ctx, schema, helper)
	if err != nil {
		return nil, nil
	}
	a := &ArrayPlan{typeInfo: tInfo}
	a.Comment = schema.Annotations.GetString("description")
	if a.ItemType = helper.TypeInfo(itemSchema.Meta()); a.ItemType.Unknown() {
		return nil, nil
	}
	if !a.ItemType.BuiltIn() {
		deps = append(deps, itemSchema)
	}
	return a, deps
}

func planEnum(ctx context.Context, helper *PlanningHelper, schema *Schema) (_ Plan, _ []*Schema) {
	if len(schema.Enum) == 0 {
		return nil, nil
	}

	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}

	e := &EnumPlan{typeInfo: tInfo}
	e.Comment = schema.Annotations.GetString("description")
	e.BaseType = TypeInfo{Name: helper.Primitive(schema.ChooseType())}
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
