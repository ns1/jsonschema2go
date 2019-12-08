package planning

import (
	"context"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"github.com/jwilner/jsonschema2go/pkg/schema"
)

func planEnum(ctx context.Context, helper generate.Helper, schema *schema.Schema) (generate.Plan, error) {
	if len(schema.Enum) == 0 {
		return nil, fmt.Errorf("no enum values: %w", ErrContinue)
	}

	tInfo := helper.TypeInfo(schema)
	if tInfo.Unknown() {
		return nil, fmt.Errorf("type unknown: %w", ErrContinue)
	}

	e := &EnumPlan{TypeInfo: tInfo, id: schema.CalcID}
	e.Comment = schema.Annotations.GetString("description")
	e.BaseType = generate.TypeInfo{Name: helper.Primitive(schema.ChooseType())}
	for _, m := range schema.Enum {
		name := helper.JSONPropertyExported(fmt.Sprintf("%s", m))
		e.Members = append(e.Members, EnumMember{Name: name, Field: m})
	}
	return e, nil
}
