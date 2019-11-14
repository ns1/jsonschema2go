package jsonschema2go

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var Composite = CompositePlanner{
	plannerFunc(planAllOfObject),
	plannerFunc(planSimpleObject),
	plannerFunc(planSimpleArray),
	plannerFunc(planEnum),
	plannerFunc(planDiscriminatedOneOfObject),
}

type Planner interface {
	Plan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error)
}

type CompositePlanner []Planner

func (c CompositePlanner) Plan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	for _, p := range c {
		pl, err := p.Plan(ctx, helper, schema)
		if err != nil {
			return nil, err
		}
		if pl != nil {
			return pl, nil
		}
	}
	return nil, errors.New("unable to plan")
}

type plannerFunc func(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error)

func (p plannerFunc) Plan(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	return p(ctx, helper, schema)
}

type PlanningHelper struct {
	Loader
	typer
	Deps      chan *Schema
	submitted <-chan struct{}
}

func (p *PlanningHelper) Schemas() <-chan *Schema {
	return p.Deps
}

func (p *PlanningHelper) Submitted() <-chan struct{} {
	return p.submitted
}

func (p *PlanningHelper) Dep(ctx context.Context, schemas ...*Schema) error {
	for _, s := range schemas {
		select {
		case p.Deps <- s:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func planDiscriminatedOneOfObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	discrim := schema.Config.Discriminator
	if !discrim.isSet() {
		return nil, nil
	}
	composedTyp, schemas, err := loadSchemaList(ctx, helper, schema, schema.OneOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 || composedTyp != Object {
		return nil, nil
	}

	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}

	typeToNames := make(map[string][]string)
	for k, v := range discrim.Mapping {
		typeToNames[v] = append(typeToNames[v], k)
	}

	typeMapping := make(map[string]TypeInfo)
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")
	for _, subSchema := range schemas {
		tInfo := helper.TypeInfo(subSchema.Meta())
		names, ok := typeToNames[tInfo.Name]
		if !ok {
			return nil, fmt.Errorf("no discriminators for type: %v", tInfo.Name)
		}
		for _, n := range names {
			typeMapping[n] = tInfo
		}
	}
	if err := helper.Dep(ctx, schemas...); err != nil {
		return nil, err
	}

	s.Fields = append(s.Fields, StructField{
		Names: []string{jsonPropertyToExportedName(discrim.PropertyName)},
		Type:  TypeInfo{Name: "interface{}"},
	})

	s.Traits = append(s.Traits,
		&discriminatorMarshalTrait{
			StructField{
				Names: []string{jsonPropertyToExportedName(discrim.PropertyName)},
				Type:  TypeInfo{Name: "string"},
				Tag:   fmt.Sprintf(`json:"%s"`, discrim.PropertyName),
			},
			typeMapping,
		},
	)

	return s, nil
}

type discriminatorMarshalTrait struct {
	StructField
	types map[string]TypeInfo
}

func (d *discriminatorMarshalTrait) Template() string {
	return "discriminator.tmpl"
}

type DiscriminatorCase struct {
	Value string
	TypeInfo
}

func (d *discriminatorMarshalTrait) Cases() (cases []DiscriminatorCase) {
	for k, v := range d.types {
		cases = append(cases, DiscriminatorCase{Value: k, TypeInfo: v})
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Name < cases[j].Name
	})
	return cases
}

func (d *discriminatorMarshalTrait) Deps() []TypeInfo {
	return []TypeInfo{{GoPath: "encoding/json", Name: "Marshal"}, {GoPath: "fmt", Name: "Errorf"}}
}

func loadSchemaList(
	ctx context.Context,
	helper *PlanningHelper,
	parent *Schema,
	schemas []*RefOrSchema,
) (SimpleType, []*Schema, error) {
	var (
		resolved  []*Schema
		foundType SimpleType
	)
	for _, s := range schemas {
		r, err := s.Resolve(ctx, parent, helper)
		if err != nil {
			return Unknown, nil, err
		}
		resolved = append(resolved, r)
		t := r.ChooseType()
		if t == Unknown {
			continue
		}
		if foundType == Unknown {
			foundType = t
			continue
		}
	}
	return foundType, resolved, nil
}

func planAllOfObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	composedTyp, schemas, err := loadSchemaList(ctx, helper, schema, schema.AllOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 || composedTyp != Object {
		return nil, nil
	}

	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")

	for _, subSchema := range schemas {
		tInfo := helper.TypeInfo(subSchema.Meta())
		if tInfo.Unknown() {
			// this is an anonymous struct; add all of its inner fields to parent
			fields, err := deriveStructFields(ctx, helper, subSchema)
			if err != nil {
				return nil, err
			}
			s.Fields = append(s.Fields, fields...)
			continue
		}
		// this is a named type, add an embedded field for the subschema type
		s.Fields = append(s.Fields, StructField{Type: tInfo})
		if err := helper.Dep(ctx, subSchema); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func planSimpleObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	if schema.ChooseType() != Object {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo}
	s.Comment = schema.Annotations.GetString("description")
	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields
	return s, nil
}

func planSimpleArray(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	if schema.ChooseType() != Array || schema.Items == nil || schema.Items.Items == nil {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema.Meta())
	if tInfo.Unknown() {
		return nil, nil
	}

	itemSchema, err := schema.Items.Items.Resolve(ctx, schema, helper)
	if err != nil {
		return nil, err
	}
	a := &ArrayPlan{typeInfo: tInfo}
	a.Comment = schema.Annotations.GetString("description")
	if a.ItemType = helper.TypeInfo(itemSchema.Meta()); a.ItemType.Unknown() {
		return nil, nil
	}
	if !a.ItemType.BuiltIn() {
		if err := helper.Dep(ctx, itemSchema); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func planEnum(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
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

func deriveStructFields(
	ctx context.Context,
	helper *PlanningHelper,
	schema *Schema,
) (fields []StructField, _ error) {
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

		fType := helper.TypeInfo(fieldSchema.Meta())
		if fType.Unknown() && len(fieldSchema.OneOf) == 2 {
			oneOfA, err := fieldSchema.OneOf[0].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			oneOfB, err := fieldSchema.OneOf[1].Resolve(ctx, fieldSchema, helper)
			if err != nil {
				return nil, err
			}
			if oneOfA.ChooseType() == Null || oneOfB.ChooseType() == Null {
				// this is a nillable field
				valueSchema := oneOfA
				if valueSchema.ChooseType() == Null {
					valueSchema = oneOfB
				}
				if fType = helper.TypeInfo(valueSchema.Meta()); fType.Unknown() {
					return nil, nil
				}
				fType.Pointer = true
			}
		}
		if fType.Unknown() {
			return nil, nil
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
			if err := helper.Dep(ctx, fieldSchema); err != nil {
				return nil, err
			}
		}
	}
	return
}

var defaultTyper = typer{defaultTypeFunc, primitives}

func defaultTypeFunc(s SchemaMeta) TypeInfo {
	parts := strings.SplitN(s.Flags.GoPath, "#", 2)
	if len(parts) == 2 {
		return TypeInfo{GoPath: parts[0], Name: parts[1]}
	}
	return TypeInfo{}
}

type typer struct {
	typeFunc   func(SchemaMeta) TypeInfo
	primitives map[SimpleType]string
}

func (d typer) TypeInfo(s SchemaMeta) TypeInfo {
	if f := d.typeFunc(s); f.Name != "" {
		return f
	}
	return TypeInfo{Name: d.Primitive(s.BestType)}
}

func (d typer) Primitive(s SimpleType) string {
	return d.primitives[s]
}
