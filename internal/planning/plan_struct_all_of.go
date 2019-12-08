package planning

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
)

func planAllOfObject(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	composedTyp, schemas, err := loadSchemaList(ctx, helper, schema, schema.AllOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no allOf schemas: %w", ErrContinue)
	}
	if composedTyp != sch.Object {
		return nil, fmt.Errorf("not an object: %w", ErrContinue)
	}
	tInfo := helper.TypeInfoHinted(schema, composedTyp)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", ErrContinue)
	}
	// we've matched

	s := &StructPlan{TypeInfo: tInfo, Id: schema.CalcID}
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
		s.Fields = append(s.Fields, StructField{Type: tInfo, FieldValidators: []Validator{SubschemaValidator}})
		if err := helper.Dep(ctx, subSchema); err != nil {
			return nil, err
		}
	}

	for _, f := range s.Fields {
		if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
			s.Traits = append(s.Traits, &BoxedEncodingTrait{})
			break
		}
	}

	return s, nil
}
