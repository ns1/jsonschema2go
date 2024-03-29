package composite

import (
	"context"
	"fmt"
	"sort"

	"github.com/ns1/jsonschema2go/internal/validator"
	"github.com/ns1/jsonschema2go/pkg/gen"
)

// PlanOneOfDiffTypes attempts to generate a plan for a schema which is a `oneOf` with multiple schemas of different
// primitive JSON types. Because there is no way of discriminating between values of the same type, all values must be
// of different types.
func PlanOneOfDiffTypes(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	_, schemas, err := loadSchemaList(ctx, helper, schema, schema.OneOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no oneOf schemas: %w", gen.ErrContinue)
	}
	seen := make(map[gen.JSONType]bool)
	for _, s := range schemas {
		typ, err := helper.DetectSimpleType(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("unable to detect simple type for %v: %w", s, err)
		}
		if typ == gen.JSONInteger {
			typ = gen.JSONNumber // we cannot be guaranteed to distinguish between floats and ints, so treat same
		}
		if seen[typ] {
			return nil, fmt.Errorf("type %v seen too many times: %w", typ, gen.ErrContinue)
		}
		seen[typ] = true
	}
	tInfo := helper.TypeInfoHinted(schema, gen.JSONObject)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("schema type is unknown: %w", gen.ErrContinue)
	}

	s := &StructPlan{TypeInfo: tInfo, ID: schema.ID}
	s.Comment = schema.Annotations.GetString("description")

	f := StructField{Name: "Value", Type: gen.TypeInfo{Name: "interface{}"}}

	var (
		trait            marshalOneOfTrait
		checkedSubSchema bool
	)
	for _, subSchema := range schemas {
		info, err := helper.TypeInfo(subSchema)
		if err != nil {
			return nil, err
		}
		if !info.BuiltIn() {
			if err := helper.Dep(ctx, subSchema); err != nil {
				return nil, err
			}
		}

		jType, err := helper.DetectSimpleType(ctx, subSchema)
		if err != nil {
			return nil, fmt.Errorf("unable to choose simple type for %v: %w", subSchema, err)
		}

		// if any one of the subschema are nullable, then the entire one-of block is "nullable"
		if subSchema.Nullable {
			trait.Nil = true
		}

		switch jType {
		case gen.JSONObject:
			trait.Object = info
		case gen.JSONArray:
			trait.Array = info
		case gen.JSONString:
			trait.Primitives = append(trait.Primitives, "string")
		case gen.JSONNumber:
			trait.Primitives = append(trait.Primitives, "float64")
		case gen.JSONInteger:
			trait.Primitives = append(trait.Primitives, "int64")
		case gen.JSONBoolean:
			trait.Primitives = append(trait.Primitives, "bool")
		case gen.JSONNull:
			trait.Nil = true
		}

		for _, v := range validator.Validators(subSchema) {
			if v.Name == validator.SubschemaValidator.Name {
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
	Object     gen.TypeInfo
	Array      gen.TypeInfo
	Primitives []string
	Nil        bool
}

func (m marshalOneOfTrait) Template() string {
	return "oneOf"
}

func (m marshalOneOfTrait) Deps() []gen.TypeInfo {
	return []gen.TypeInfo{
		{GoPath: "encoding/json", Name: "NewEncoder"},
		{GoPath: "encoding/json", Name: "Marshal"},
		{GoPath: "encoding/json", Name: "Delim"},
		{GoPath: "fmt", Name: "Errorf"},
		{GoPath: "bytes", Name: "NewReader"},
	}
}

type discriminatorMarshalTrait struct {
	StructField
	types map[string]gen.TypeInfo
}

func (d *discriminatorMarshalTrait) Template() string {
	return "discriminator"
}

type discriminatorCase struct {
	Value string
	gen.TypeInfo
}

func (d *discriminatorMarshalTrait) Default() *discriminatorCase {
	for k, v := range d.types {
		if k == "*" {
			return &discriminatorCase{Value: k, TypeInfo: v}
		}
	}
	return nil
}

func (d *discriminatorMarshalTrait) Cases() (cases []discriminatorCase) {
	for k, v := range d.types {
		if k != "*" {
			cases = append(cases, discriminatorCase{Value: k, TypeInfo: v})
		}
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Name < cases[j].Name
	})
	return cases
}

func (d *discriminatorMarshalTrait) Deps() []gen.TypeInfo {
	return []gen.TypeInfo{{GoPath: "encoding/json", Name: "Marshal"}, {GoPath: "fmt", Name: "Errorf"}}
}

// Trait encapsulates customization of the struct's behavior (usually around serialization and deserialization)
type Trait interface {
	Template() string
}
