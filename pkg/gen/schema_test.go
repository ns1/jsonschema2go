package gen

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_getJSONFieldNames(t *testing.T) {
	tests := []struct {
		name       string
		val        interface{}
		wantFields []string
	}{
		{
			name: "no fields",
			val:  struct{}{},
		},
		{
			name: "no tag",
			val: struct {
				Name string
			}{},
			wantFields: []string{"Name"},
		},
		{
			name: "empty tag",
			val: struct {
				Name string `json:""`
			}{},
			wantFields: []string{"Name"},
		},
		{
			name: "other tag",
			val: struct {
				Name string `bson:"blahblah"`
			}{},
			wantFields: []string{"Name"},
		},
		{
			name: "skip",
			val: struct {
				Name string `json:"-"`
			}{},
		},
		{
			name: "filled omitempty",
			val: struct {
				Name  string `json:"name,omitempty"`
				Other string `json:"other,omitempty"`
			}{},
			wantFields: []string{"name", "other"},
		},
		{
			name: "empty omitempty",
			val: struct {
				Name string `json:",omitempty"`
			}{},
			wantFields: []string{"Name"},
		},
		{
			name: "schema",
			val:  Schema{},
			wantFields: []string{
				"$ref",
				"$schema",
				"multipleOf",
				"maximum",
				"exclusiveMaximum",
				"minimum",
				"exclusiveMinimum",
				"maxLength",
				"minLength",
				"pattern",
				"additionalItems",
				"items",
				"maxItems",
				"minItems",
				"uniqueItems",
				"maxProperties",
				"minProperties",
				"required",
				"additionalProperties",
				"definitions",
				"properties",
				"patternProperties",
				"dependencies",
				"enum",
				"type",
				"format",
				"allOf",
				"anyOf",
				"oneOf",
				"not",
				"x-jsonschema2go",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantFields, getJSONFieldNames(tt.val))
		})
	}
}

func TestSchema_UnmarshalJSON(t *testing.T) {
	strPtr := func(s string) *string {
		return &s
	}
	boolPtr := func(b bool) *bool {
		return &b
	}

	tests := []struct {
		name    string
		data    string
		wantErr bool
		want    Schema
	}{
		{
			name:    "empty",
			data:    ``,
			wantErr: true,
		},
		{
			name: "empty obj",
			data: `{}`,
		},
		{
			name: "ref",
			data: `{"$ref": "#"}`,
			want: Schema{Ref: strPtr("#")},
		},
		{
			name: "simple",
			data: `{"type": "string"}`,
			want: Schema{Type: &TypeField{String}},
		},
		{
			name: "simple",
			data: `{"type": "array", "items": {"type": "string"}}`,
			want: Schema{Type: &TypeField{Array}, Items: &ItemsField{Items: schema(Schema{Type: &TypeField{String}})}},
		},
		{
			name: "annos",
			data: `{"type": "string", "i-am-an-annotation": "hi"}`,
			want: Schema{Type: &TypeField{String}, Annotations: annos(map[string]string{"i-am-an-annotation": "hi"})},
		},
		{
			name: "recursive",
			data: `{"not": {"$ref": "https://somewhereelse"}}`,
			want: Schema{Not: ref("https://somewhereelse")},
		},
		{
			name: "allOf",
			data: `{"allOf": [{"$ref": "https://somewhereelse"}]}`,
			want: Schema{AllOf: []*RefOrSchema{ref("https://somewhereelse")}},
		},
		{
			name: "additionalProperties bool",
			data: `{"allOf": [{"additionalProperties": true}]}`,
			want: Schema{AllOf: []*RefOrSchema{
				schema(Schema{AdditionalProperties: &BoolOrSchema{Bool: boolPtr(true)}}),
			}},
		},
		{
			name: "itemFields tuple",
			data: `{"items": [{"type": "integer"}, {"type": "string"}]}`,
			want: Schema{
				Items: &ItemsField{
					TupleFields: []*RefOrSchema{
						schema(Schema{Type: &TypeField{Integer}}),
						schema(Schema{Type: &TypeField{String}}),
					},
				},
			},
		},
		{
			name: "itemFields schema",
			data: `{"items": {"type": "integer"}}`,
			want: Schema{
				Items: &ItemsField{
					Items: schema(Schema{Type: &TypeField{Integer}}),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Schema
			if err := r.UnmarshalJSON([]byte(tt.data)); (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, tt.want, r)
		})
	}
}

func schema(s Schema) *RefOrSchema {
	return &RefOrSchema{schema: &s}
}

func ref(s string) *RefOrSchema {
	return &RefOrSchema{ref: &s}
}

func annos(annos map[string]string) TagMap {
	m := make(map[string]json.RawMessage)
	for k, v := range annos {
		m[k], _ = json.Marshal(v)
	}
	return m
}
