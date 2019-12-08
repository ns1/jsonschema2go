package planning

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"sort"
)

func planOneOfDiffTypes(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	_, schemas, err := loadSchemaList(ctx, helper, schema, schema.OneOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no oneOf schemas: %w", ErrContinue)
	}
	seen := make(map[sch.SimpleType]bool)
	for _, s := range schemas {
		typ := s.ChooseType()
		if typ == sch.Integer {
			typ = sch.Number // we cannot be guaranteed to distinguish between floats and ints, so treat same
		}
		if seen[typ] {
			return nil, fmt.Errorf("type %v seen too many times: %w", typ, ErrContinue)
		}
		seen[typ] = true
	}
	tInfo := helper.TypeInfoHinted(schema, sch.Object)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("schema type is unknown: %w", ErrContinue)
	}

	s := &StructPlan{TypeInfo: tInfo, Id: schema.CalcID}
	s.Comment = schema.Annotations.GetString("description")

	f := StructField{Name: "Value", Type: generate.TypeInfo{Name: "interface{}"}}

	var (
		trait            marshalOneOfTrait
		checkedSubSchema bool
	)
	for _, subSchema := range schemas {
		info := helper.TypeInfo(subSchema)
		if !info.BuiltIn() {
			if err := helper.Dep(ctx, subSchema); err != nil {
				return nil, err
			}
		}

		switch subSchema.ChooseType() {
		case sch.Object:
			trait.Object = info
		case sch.Array:
			trait.Array = info
		case sch.String:
			trait.Primitives = append(trait.Primitives, "string")
		case sch.Number:
			trait.Primitives = append(trait.Primitives, "float64")
		case sch.Integer:
			trait.Primitives = append(trait.Primitives, "int64")
		case sch.Boolean:
			trait.Primitives = append(trait.Primitives, "bool")
		case sch.Null:
			trait.Nil = true
		}

		for _, v := range validators(subSchema) {
			if v.Name == SubschemaValidator.Name {
				if checkedSubSchema {
					continue
				}
				checkedSubSchema = true
			}
			f.FieldValidators = append(f.FieldValidators, v)
		}
	}
	s.Traits = []Trait{trait}
	s.Fields = []StructField{f}
	return s, nil
}

type marshalOneOfTrait struct {
	Object     generate.TypeInfo
	Array      generate.TypeInfo
	Primitives []string
	Nil        bool
}

func (m marshalOneOfTrait) Template() string {
	return "oneOf"
}

func (m marshalOneOfTrait) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{
		{GoPath: "encoding/json", Name: "NewEncoder"},
		{GoPath: "encoding/json", Name: "Marshal"},
		{GoPath: "encoding/json", Name: "Delim"},
		{GoPath: "fmt", Name: "Errorf"},
		{GoPath: "bytes", Name: "NewReader"},
	}
}

type discriminatorMarshalTrait struct {
	StructField
	types map[string]generate.TypeInfo
}

func (d *discriminatorMarshalTrait) Template() string {
	return "discriminator"
}

type DiscriminatorCase struct {
	Value string
	generate.TypeInfo
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

func (d *discriminatorMarshalTrait) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{{GoPath: "encoding/json", Name: "Marshal"}, {GoPath: "fmt", Name: "Errorf"}}
}
