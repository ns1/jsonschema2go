package composite

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/gen"
)

// PlanDiscriminatedOneOfObject generates a value for a oneOf array of object schemas, provided discriminator config
// has been provided; if it doesn't match, ErrContinue is returned.
func PlanDiscriminatedOneOfObject(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	discrim := schema.Config.Discriminator
	if !discrim.IsSet() {
		return nil, fmt.Errorf("discriminator is not set: %w", gen.ErrContinue)
	}
	composedTyp, schemas, err := loadSchemaList(ctx, helper, schema, schema.OneOf)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no schemas: %w", gen.ErrContinue)
	}
	if composedTyp != gen.Object {
		return nil, fmt.Errorf("composed type is not object: %w", gen.ErrContinue)
	}

	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("schema type is unknown: %w", gen.ErrContinue)
	}

	typeToNames := make(map[string][]string)
	for k, v := range discrim.Mapping {
		typeToNames[v] = append(typeToNames[v], k)
	}

	typeMapping := make(map[string]gen.TypeInfo)
	s := &StructPlan{TypeInfo: tInfo, ID: schema.CalcID}
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
		Type:     gen.TypeInfo{Name: "interface{}"},
	})

	s.Traits = append(s.Traits,
		&discriminatorMarshalTrait{
			StructField{
				Name:     helper.JSONPropertyExported(discrim.PropertyName),
				JSONName: discrim.PropertyName,
				Type:     gen.TypeInfo{Name: "string"},
				Tag:      fmt.Sprintf(`json:"%s"`, discrim.PropertyName),
			},
			typeMapping,
		},
	)

	return s, nil
}
