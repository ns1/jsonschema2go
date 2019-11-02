package jsonschema2go

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func TestLoad(t *testing.T) {
	strPtr := func(s string) *string {
		return &s
	}

	for _, tt := range []struct {
		name    string
		schemas map[string]*Schema
		path    string
		want    *Schema
		wantErr string
	}{
		{
			name: "simple",
			schemas: map[string]*Schema{
				"/hi.json": {Type: &TypeField{String}},
			},
			path: "file:/hi.json",
			want: &Schema{Type: &TypeField{String}},
		},
		{
			name: "resolution",
			schemas: map[string]*Schema{
				"/bob/parent.json": {
					Type: &TypeField{String},
					Properties: map[string]*Schema{
						"bob": {Not: &Schema{Ref: strPtr("../child.json")}},
					},
				},
				"/child.json": {Type: &TypeField{Integer}},
			},
			path: "file:/bob/parent.json",
			want: &Schema{
				Type: &TypeField{String},
				Properties: map[string]*Schema{
					"bob": {Not: &Schema{Type: &TypeField{Integer}}},
				},
			},
		},
		{
			name: "deeper",
			schemas: map[string]*Schema{
				"/bob/parent.json": {
					Type: &TypeField{String},
					Properties: map[string]*Schema{
						"bob": {Not: &Schema{Ref: strPtr("../child.json")}},
					},
					AdditionalProperties: &BoolOrSchema{
						Schema: &Schema{
							Ref: strPtr("foo/bar.json"),
						},
					},
				},
				"/child.json":       {Type: &TypeField{Integer}},
				"/bob/foo/bar.json": {Type: &TypeField{Object}},
			},
			path: "file:/bob/parent.json",
			want: &Schema{
				Type: &TypeField{String},
				Properties: map[string]*Schema{
					"bob": {Not: &Schema{Type: &TypeField{Integer}}},
				},
				AdditionalProperties: &BoolOrSchema{
					Schema: &Schema{Type: &TypeField{Object}},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			found, err := newResolver(mockLoader(tt.schemas)).Resolve(context.Background(), tt.path)
			{
				var errS string
				if err != nil {
					errS = err.Error()
				}
				if (tt.wantErr == "") != (errS == "") {
					t.Fatalf("wanted err %q but got %q", tt.wantErr, errS)
				}
			}

			require.Equal(t, tt.want, found)
		})
	}
}

type mockLoader map[string]*Schema

func (m mockLoader) Load(ctx context.Context, s string) (*Schema, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "file" || u.Host != "" {
		return nil, fmt.Errorf("expected \"file\" scheme and no host but got %q and %q: %q", u.Scheme, u.Host, s)
	}
	v, ok := m[u.Path]
	if !ok {
		return nil, fmt.Errorf("%q not found", s)
	}
	return v, nil
}

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
				"id",
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
			want: Schema{Type: &TypeField{Array}, Items: &ItemsFields{Items: &Schema{Type: &TypeField{String}}}},
		},
		{
			name: "annos",
			data: `{"type": "string", "i-am-an-annotation": "hi"}`,
			want: Schema{Type: &TypeField{String}, Annotations: map[string]interface{}{"i-am-an-annotation": "hi"}},
		},
		{
			name: "recursive",
			data: `{"not": {"$ref": "https://somewhereelse"}}`,
			want: Schema{Not: &Schema{Ref: strPtr("https://somewhereelse")}},
		},
		{
			name: "allOf",
			data: `{"allOf": [{"$ref": "https://somewhereelse"}]}`,
			want: Schema{AllOf: []*Schema{{Ref: strPtr("https://somewhereelse")}}},
		},
		{
			name: "additionalProperties bool",
			data: `{"allOf": [{"additionalProperties": true}]}`,
			want: Schema{AllOf: []*Schema{{AdditionalProperties: &BoolOrSchema{Bool: boolPtr(true)}}}},
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
