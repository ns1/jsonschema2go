package jsonschema2go

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchemaToPlan(t *testing.T) {
	tests := []struct {
		name    string
		schema  *Schema
		want    []Plan
		wantErr bool
	}{
		{
			name: "simple",
			schema: &Schema{
				Type: &TypeField{Object},
				Properties: map[string]*Schema{
					"count": {Type: &TypeField{Integer}},
				},
				Annotations: map[string]interface{}{
					"description":      "i am bob",
					"x-go-import-path": "github.com/jwilner/jsonschema2go/example#Awesome",
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Names: []string{"count"},
							Type:  BuiltInInt,
							Tag:   `json="count,omitempty"`,
						},
					},
				},
			},
		},
		{
			name: "nested struct",
			schema: &Schema{
				Type: &TypeField{Object},
				Properties: map[string]*Schema{
					"nested": {
						Annotations: map[string]interface{}{
							"x-go-import-path": "github.com/jwilner/jsonschema2go/example#NestedType",
						},
						Type: &TypeField{Object},
						Properties: map[string]*Schema{
							"count": {Type: &TypeField{Integer}},
						},
					},
				},
				Annotations: map[string]interface{}{
					"description":      "i am bob",
					"x-go-import-path": "github.com/jwilner/jsonschema2go/example#Awesome",
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Names: []string{"nested"},
							Type: TypeInfo{
								GoPath:   "github.com/jwilner/jsonschema2go/example",
								Name:     "NestedType",
								FileName: "values.gen.go",
							},
							Tag: `json="nested,omitempty"`,
						},
					},
				},
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "NestedType",
					},
					Fields: []StructField{
						{
							Names: []string{"count"},
							Type:  BuiltInInt,
							Tag:   `json="count,omitempty"`,
						},
					},
				},
			},
		},
		{
			name: "composed anonymous struct",
			schema: &Schema{
				Annotations: map[string]interface{}{
					"x-go-import-path": "github.com/jwilner/jsonschema2go/example#AwesomeWithID",
				},
				AllOf: []*Schema{
					{
						Type: &TypeField{Object},
						Properties: map[string]*Schema{
							"id": {Type: &TypeField{Integer}},
						},
					},
					{
						Type: &TypeField{Object},
						Properties: map[string]*Schema{
							"count": {Type: &TypeField{Integer}},
						},
						Annotations: map[string]interface{}{
							"description":      "i am bob",
							"x-go-import-path": "github.com/jwilner/jsonschema2go/example#Awesome",
						},
					},
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "AwesomeWithID",
					},
					Fields: []StructField{
						{
							Names: []string{"id"},
							Type:  BuiltInInt,
							Tag:   `json="id,omitempty"`,
						},
						{
							Type: TypeInfo{
								FileName: "values.gen.go",
								GoPath:   "github.com/jwilner/jsonschema2go/example",
								Name:     "Awesome",
							},
						},
					},
				},
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Names: []string{"count"},
							Type:  BuiltInInt,
							Tag:   `json="count,omitempty"`,
						},
					},
				},
			},
		},
		{
			name: "enum",
			schema: &Schema{
				Annotations: map[string]interface{}{
					"x-go-import-path": "github.com/jwilner/jsonschema2go/example#Letter",
				},
				Type: &TypeField{String},
				Enum: []interface{}{
					"A",
					"B",
					"C",
				},
			},
			want: []Plan{
				&EnumPlan{
					typeInfo: TypeInfo{
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "Letter",
						FileName: "values.gen.go",
					},
					BaseType: BuiltInString,
					Members: []interface{}{
						"A",
						"B",
						"C",
					},
				},
			},
		},
		{
			name: "nullable built in",
			schema: &Schema{
				Annotations: map[string]interface{}{
					"x-go-import-path": "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Type: &TypeField{Object},
				Properties: map[string]*Schema{
					"bob": {
						OneOf: []*Schema{
							{Type: &TypeField{Null}},
							{Type: &TypeField{Integer}},
						},
					},
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						FileName: "values.gen.go",
						GoPath:   "github.com/jwilner/jsonschema2go/example",
						Name:     "Awesome",
					},
					Fields: []StructField{
						{
							Names: []string{"bob"},
							Type:  BuiltInIntPointer,
							Tag:   `json="bob,omitempty"`,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SchemaToPlan(tt.schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("SchemaToPlan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
