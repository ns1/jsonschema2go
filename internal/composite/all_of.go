package composite

import (
	"context"
	"fmt"
	"github.com/ns1/jsonschema2go/internal/validator"
	"github.com/ns1/jsonschema2go/pkg/gen"
	"sort"
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
	if composedTyp != gen.JSONObject {
		return nil, fmt.Errorf("not an object: %w", gen.ErrContinue)
	}
	tInfo := helper.TypeInfoHinted(schema, composedTyp)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", gen.ErrContinue)
	}
	// we've matched

	s := &StructPlan{TypeInfo: tInfo, ID: schema.ID}
	s.Comment = schema.Annotations.GetString("description")

	fields, err := deriveStructFields(ctx, helper, schema)
	if err != nil {
		return nil, err
	}
	s.Fields = fields

	required := make(map[string]bool, len(schema.Required))
	for _, k := range schema.Required {
		required[k] = true
	}
	subRequiredFields := make(map[string]StructField)
	setSubRequired := func(flds []StructField) {
		for _, f := range flds {
			if required[f.JSONName] {
				subRequiredFields[f.JSONName] = f
			}
		}
	}

	for _, subSchema := range schemas {
		fields, err := deriveStructFields(ctx, helper, subSchema)
		if err != nil {
			return nil, err
		}
		setSubRequired(fields)
		if subSchema.Config.PromoteFields {
			// this is an anonymous struct; add all of its inner fields to parent
			s.Fields = append(s.Fields, fields...)
			continue
		}

		// parent may still check whether required field present

		tInfo := helper.TypeInfoHinted(subSchema, gen.JSONObject)
		// this is a named type, add an embedded field for the subschema type
		s.Fields = append(s.Fields, StructField{Type: tInfo, FieldValidators: []validator.Validator{validator.SubschemaValidator}})
		if err := helper.Dep(ctx, subSchema); err != nil {
			return nil, err
		}
	}

	keys := make([]string, 0, len(subRequiredFields))
	for k := range subRequiredFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := subRequiredFields[k]
		v.Required = true
		s.SubRequired = append(s.SubRequired, v)
	}

	return s, nil
}
