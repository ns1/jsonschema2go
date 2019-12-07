package jsonschema2go

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

var (
	Composite = CompositePlanner{
		plannerFunc(planAllOfObject),
		plannerFunc(planSimpleObject),
		plannerFunc(planSimpleArray),
		plannerFunc(planEnum),
		plannerFunc(planDiscriminatedOneOfObject),
	}
	subschemaValidator = Validator{Name: "subschema"}
)

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
	// we require types for objects and arrays
	if t := schema.ChooseType(); t == Object || t == Array {
		id := schema.Loc
		if schema.CalcID != nil {
			id = schema.CalcID
		}
		return nil, fmt.Errorf("unable to plan %v", id)
	}
	return nil, nil
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

	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, nil
	}

	typeToNames := make(map[string][]string)
	for k, v := range discrim.Mapping {
		typeToNames[v] = append(typeToNames[v], k)
	}

	typeMapping := make(map[string]TypeInfo)
	s := &StructPlan{typeInfo: tInfo, id: schema.CalcID}
	s.Comment = schema.Annotations.GetString("description")
	for _, subSchema := range schemas {
		tInfo := helper.TypeInfo(subSchema)
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
		Name:     helper.JSONPropertyExported(discrim.PropertyName),
		JSONName: discrim.PropertyName,
		Type:     TypeInfo{Name: "interface{}"},
	})

	s.Traits = append(s.Traits,
		&discriminatorMarshalTrait{
			StructField{
				Name:     helper.JSONPropertyExported(discrim.PropertyName),
				JSONName: discrim.PropertyName,
				Type:     TypeInfo{Name: "string"},
				Tag:      fmt.Sprintf(`json:"%s"`, discrim.PropertyName),
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

func (d *discriminatorMarshalTrait) Default() *DiscriminatorCase {
	for k, v := range d.types {
		if k == "*" {
			return &DiscriminatorCase{Value: k, TypeInfo: v}
		}
	}
	return nil
}

func (d *discriminatorMarshalTrait) Cases() (cases []DiscriminatorCase) {
	for k, v := range d.types {
		if k != "*" {
			cases = append(cases, DiscriminatorCase{Value: k, TypeInfo: v})
		}
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

	tInfo := helper.TypeInfoHinted(schema, composedTyp)
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo, id: schema.CalcID}
	s.Comment = schema.Annotations.GetString("description")

	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields

	for _, subSchema := range schemas {
		tInfo := helper.TypeInfo(subSchema)
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
		s.Fields = append(s.Fields, StructField{Type: tInfo, validators: []Validator{subschemaValidator}})
		if err := helper.Dep(ctx, subSchema); err != nil {
			return nil, err
		}
	}

	for _, f := range s.Fields {
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
			s.Traits = append(s.Traits, &boxedEncodingTrait{})
			break
		}
	}

	return s, nil
}

func planSimpleObject(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	if schema.ChooseType() != Object {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, nil
	}
	s := &StructPlan{typeInfo: tInfo, id: schema.CalcID}
	s.Comment = schema.Annotations.GetString("description")
	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields

	for _, f := range fields {
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
			s.Traits = append(s.Traits, &boxedEncodingTrait{})
			break
		}
	}

	return s, nil
}

func planSimpleArray(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	if schema.ChooseType() != Array {
		return nil, nil
	}
	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, nil
	}

	var itemSchema *Schema
	if schema.Items != nil && schema.Items.Items != nil {
		var err error
		if itemSchema, err = schema.Items.Items.Resolve(ctx, schema, helper); err != nil {
			return nil, err
		}
	}
	a := &ArrayPlan{typeInfo: tInfo, id: schema.CalcID}
	a.Comment = schema.Annotations.GetString("description")
	if itemSchema != nil {
		if a.ItemType = helper.TypeInfo(itemSchema); a.ItemType.Unknown() {
			return nil, nil
		}
	} else {
		a.ItemType = TypeInfo{Name: "interface{}"}
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
			testExpr: templateStr(`len({{ .QualifiedName }}) < ` + minItemsS),
			sprintfExpr: templateStr(
				`"must have length greater than ` + minItemsS + ` but was %d", len({{ .QualifiedName }})`,
			),
		})
	}
	if schema.MaxItems != nil {
		maxItemsS := strconv.FormatUint(*schema.MaxItems, 10)
		a.validators = append(a.validators, Validator{
			Name:     "maxItems",
			testExpr: templateStr(`len({{ .QualifiedName }}) > ` + maxItemsS),
			sprintfExpr: templateStr(
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
	return a, nil
}

func planEnum(ctx context.Context, helper *PlanningHelper, schema *Schema) (Plan, error) {
	if len(schema.Enum) == 0 {
		return nil, nil
	}

	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, nil
	}

	e := &EnumPlan{typeInfo: tInfo, id: schema.CalcID}
	e.Comment = schema.Annotations.GetString("description")
	e.BaseType = TypeInfo{Name: helper.Primitive(schema.ChooseType())}
	for _, m := range schema.Enum {
		name := helper.JSONPropertyExported(fmt.Sprintf("%s", m))
		e.Members = append(e.Members, EnumMember{Name: name, Field: m})
	}
	return e, nil
}

func newNamer(knownInitialisms []string) *namer {
	m := make(map[string]bool)
	for _, n := range knownInitialisms {
		m[n] = true
	}
	return &namer{m}
}

type namer struct {
	knownInitialisms map[string]bool
}

func (n *namer) JSONPropertyExported(name string) string {
	if strings.ToUpper(name) == name {
		return n.exportedIdentifier([][]rune{[]rune(strings.ToLower(name))})
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

	return n.exportedIdentifier(parts)
}

func (n *namer) exportedIdentifier(parts [][]rune) string {
	var words []string
	for _, rs := range parts {
		if word := string(rs); n.knownInitialisms[word] {
			words = append(words, strings.ToUpper(word))
			continue
		}
		rs[0] = unicode.ToUpper(rs[0])
		words = append(words, string(rs))
	}
	return strings.Join(words, "")
}

func templateStr(str string) *template.Template {
	return template.Must(template.New("").Parse(str))
}

func validators(schema *Schema) (styles []Validator) {
	switch schema.ChooseType() {
	case Array, Object:
		if !schema.Config.NoValidate {
			styles = append(styles, subschemaValidator)
		}
	case String:
		if schema.Pattern != nil {
			pattern := *schema.Pattern
			styles = append(styles, Validator{
				Name:        "pattern",
				varExpr:     templateStr("{{ .NameSpace }}Pattern = regexp.MustCompile(`" + pattern + "`)"),
				testExpr:    templateStr("!{{ .NameSpace }}Pattern.MatchString({{ .QualifiedName }})"),
				sprintfExpr: templateStr(`"must match '` + pattern + `' but got %q", {{ .QualifiedName }}`),
				Deps:        []TypeInfo{{GoPath: "regexp", Name: "MustCompile"}},
			})
		}
		if schema.MinLength != 0 {
			lenStr := strconv.FormatUint(schema.MinLength, 10)
			styles = append(styles, Validator{
				Name:     "minLength",
				testExpr: templateStr(`len({{ .QualifiedName }}) < ` + lenStr),
				sprintfExpr: templateStr(
					`"must have length greater than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
			})
		}
		if schema.MaxLength != nil {
			lenStr := strconv.FormatUint(*schema.MaxLength, 10)
			styles = append(styles, Validator{
				Name:     "maxLength",
				testExpr: templateStr(`len({{ .QualifiedName }}) > ` + lenStr),
				sprintfExpr: templateStr(
					`"must have length less than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
			})
		}
	case Integer, Number:
		if schema.MultipleOf != nil {
			multipleOf := fmt.Sprintf("%v", *schema.MultipleOf)

			var deps []TypeInfo
			expr := templateStr(`{{ .QualifiedName }}%` + multipleOf + ` != 0`)
			if schema.ChooseType() == Number {
				deps = []TypeInfo{{GoPath: "math", Name: "Mod"}}
				expr = templateStr(`math.Mod({{ .QualifiedName }}, ` + multipleOf + `) != 0`)
			}

			styles = append(styles, Validator{
				Name:        "multipleOf",
				testExpr:    expr,
				sprintfExpr: templateStr(`"must be a multiple of ` + multipleOf + ` but was %v", {{ .QualifiedName }}`),
				Deps:        deps,
			})
		}
		numValidator := func(name, comparator, english string, limit float64, exclusive bool) {
			if exclusive {
				name += "Exclusive"
				comparator += "="
			} else {
				english += " or equal to"
			}
			sLimit := fmt.Sprintf("%v", limit)
			styles = append(styles, Validator{
				Name:        name,
				testExpr:    templateStr(`{{ .QualifiedName }} ` + comparator + sLimit),
				sprintfExpr: templateStr(`"must be ` + english + ` ` + sLimit + ` but was %v", {{ .QualifiedName }}`),
			})
		}
		if schema.Minimum != nil {
			numValidator(
				"minimum",
				"<",
				"greater than",
				*schema.Minimum,
				schema.ExclusiveMinimum != nil && *schema.ExclusiveMinimum,
			)
		}
		if schema.Maximum != nil {
			numValidator(
				"maximum",
				">",
				"less than",
				*schema.Minimum,
				schema.ExclusiveMinimum != nil && *schema.ExclusiveMinimum,
			)
		}
	}
	return
}

func deriveStructFields(
	ctx context.Context,
	helper *PlanningHelper,
	schema *Schema,
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
			if oneOfA.ChooseType() == Null || oneOfB.ChooseType() == Null {
				// this is a nillable field
				valueSchema := oneOfA
				if valueSchema.ChooseType() == Null {
					valueSchema = oneOfB
				}
				if fType = helper.TypeInfo(valueSchema); fType.Unknown() {
					return nil, nil
				}
				fType.Pointer = true
			}
		}
		if fieldSchema.ChooseType() == Unknown && fType.Unknown() {
			fType = TypeInfo{Name: "interface{}"}
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
				fType = TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "String", valPath: "String"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "int64":
				fType = TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Int64", valPath: "Int64"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "bool":
				fType = TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Bool", valPath: "Bool"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			case "float64":
				fType = TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/boxed", Name: "Float64", valPath: "Float64"}
				tag = fmt.Sprintf(`json:"%s"`, name)
			}
		}
		fields = append(
			fields,
			StructField{
				Comment:    fieldSchema.Annotations.GetString("description"),
				Name:       helper.JSONPropertyExported(name),
				JSONName:   name,
				Type:       fType,
				Tag:        tag,
				Required:   required[name],
				validators: validators(fieldSchema),
			},
		)
	}
	return
}

var defaultTyper = typer{newNamer([]string{"id", "http"}), defaultTypeFunc, primitives}

func defaultTypeFunc(s *Schema) TypeInfo {
	parts := strings.SplitN(s.Config.GoPath, "#", 2)
	if len(parts) == 2 {
		return TypeInfo{GoPath: parts[0], Name: parts[1]}
	}
	return TypeInfo{}
}

type typer struct {
	*namer
	typeFunc   func(s *Schema) TypeInfo
	primitives map[SimpleType]string
}

func (d typer) TypeInfo(s *Schema) TypeInfo {
	t := s.ChooseType()
	if t != Array && t != Object && s.Config.GoPath == "" {
		return TypeInfo{Name: d.Primitive(t)}
	}
	return d.TypeInfoHinted(s, t)
}

func (d typer) TypeInfoHinted(s *Schema, t SimpleType) TypeInfo {
	if f := d.typeFunc(s); f.Name != "" {
		f.Name = d.namer.JSONPropertyExported(f.Name)
		return f
	}
	return TypeInfo{Name: d.Primitive(t)}
}

func (d typer) Primitive(s SimpleType) string {
	return d.primitives[s]
}
