package composite

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/validator"
	"github.com/jwilner/jsonschema2go/pkg/gen"
)

// PlanAllOfObject attempts to generate a Plan from the AllOf schemas on an object; if it doesn't match, ErrContinue
// is returned.
func PlanAllOfObject(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	composedTyp, schemas, err := loadSchemaList(ctx, helper, schema, schema.AllOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no allOf schemas: %w", gen.ErrContinue)
	}
	if composedTyp != gen.Object {
		return nil, fmt.Errorf("not an object: %w", gen.ErrContinue)
	}
	tInfo := helper.TypeInfoHinted(schema, composedTyp)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", gen.ErrContinue)
	}
	// we've matched

	s := &StructPlan{TypeInfo: tInfo, ID: schema.CalcID}
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
		s.Fields = append(s.Fields, StructField{Type: tInfo, FieldValidators: []validator.Validator{validator.SubschemaValidator}})
		if err := helper.Dep(ctx, subSchema); err != nil {
			return nil, err
		}
	}

	for _, f := range s.Fields {
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/pkg/boxed" {
			s.Traits = append(s.Traits, &boxedEncodingTrait{})
			break
		}
	}

	return s, nil
}
