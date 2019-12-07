package jsonschema2go

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"sort"
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
				Properties: map[string]*RefOrSchema{
					"count": schema(Schema{Type: &TypeField{Integer}}),
				},
				Config: config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Name: "Count",
							Type: TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								valPath: "Int64",
							},
							JSONName: "count",
							Tag:      `json:"count"`,
						},
					},
					Traits: []Trait{&boxedEncodingTrait{}},
				},
			},
		},
		{
			name: "nested struct",
			schema: &Schema{
				Type: &TypeField{Object},
				Properties: map[string]*RefOrSchema{
					"nested": schema(Schema{
						Config: config{
							GoPath: "github.com/jwilner/jsonschema2go/example#NestedType",
						},
						Type: &TypeField{Object},
						Properties: map[string]*RefOrSchema{
							"count": schema(Schema{Type: &TypeField{Integer}}),
						},
					}),
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
				Config: config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Name:     "Nested",
							JSONName: "nested",
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "NestedType",
							},
							Tag:        `json:"nested,omitempty"`,
							validators: []Validator{{Name: "subschema"}},
						},
					},
				},
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "NestedType",
					},
					Fields: []StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								valPath: "Int64",
							},
							Tag: `json:"count"`,
						},
					},
					Traits: []Trait{&boxedEncodingTrait{}},
				},
			},
		},
		{
			name: "composed anonymous struct",
			schema: &Schema{
				Config: config{
					GoPath: "github.com/jwilner/jsonschema2go/example#AwesomeWithID",
				},
				AllOf: []*RefOrSchema{
					schema(
						Schema{
							Type: &TypeField{Object},
							Properties: map[string]*RefOrSchema{
								"id": schema(Schema{Type: &TypeField{Integer}}),
							},
						},
					),
					schema(
						Schema{
							Type: &TypeField{Object},
							Properties: map[string]*RefOrSchema{
								"count": schema(Schema{Type: &TypeField{Integer}}),
							},
							Annotations: annos(map[string]string{
								"description": "i am bob",
							}),
							Config: config{
								GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
							},
						},
					),
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								valPath: "Int64",
							},
							Tag: `json:"count"`,
						},
					},
					Traits: []Trait{&boxedEncodingTrait{}},
				},
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "AwesomeWithID",
					},
					Fields: []StructField{
						{
							Name:     "ID",
							JSONName: "id",
							Type: TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								valPath: "Int64",
							},
							Tag: `json:"id"`,
						},
						{
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "Awesome",
							},
							validators: []Validator{subschemaValidator},
						},
					},
					Traits: []Trait{&boxedEncodingTrait{}},
				},
			},
		},
		{
			name: "enum",
			schema: &Schema{
				Config: config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Letter",
				},
				Type: &TypeField{String},
				Enum: []interface{}{
					"a",
					"b",
					"c",
				},
			},
			want: []Plan{
				&EnumPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Letter",
					},
					BaseType: TypeInfo{Name: "string"},
					Members: []EnumMember{
						{"A", "a"},
						{"B", "b"},
						{"C", "c"},
					},
				},
			},
		},
		{
			name: "nullable built in",
			schema: &Schema{
				Config: config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Type: &TypeField{Object},
				Properties: map[string]*RefOrSchema{
					"bob": schema(Schema{
						OneOf: []*RefOrSchema{
							schema(Schema{Type: &TypeField{Null}}),
							schema(Schema{Type: &TypeField{Integer}}),
						},
					}),
				},
			},
			want: []Plan{
				&StructPlan{
					typeInfo: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Fields: []StructField{
						{
							Name:     "Bob",
							JSONName: "bob",
							Type:     TypeInfo{Name: "int64", Pointer: true},
							Tag:      `json:"bob,omitempty"`,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := crawl(context.Background(), Composite, mockLoader{}, defaultTyper, schemaChan(tt.schema))
			var (
				got []Plan
				err error
			)
			for r := range results {
				if r.Plan != nil {
					got = append(got, r.Plan)
				}
				if r.Err != nil {
					err = r.Err
				}
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("SchemaToPlan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			sort.Slice(got, func(i, j int) bool {
				if pathI, pathJ := got[i].Type().GoPath, got[j].Type().GoPath; pathI != pathJ {
					return pathI < pathJ
				}
				return got[i].Type().Name < got[j].Type().Name
			})
			require.Equal(t, tt.want, got)
		})
	}
}

func schemaChan(schemas ...*Schema) <-chan *Schema {
	schemaC := make(chan *Schema)
	go func() {
		for _, s := range schemas {
			schemaC <- s
		}
		close(schemaC)
	}()
	return schemaC
}

func annos(annos map[string]string) TagMap {
	m := make(map[string]json.RawMessage)
	for k, v := range annos {
		m[k], _ = json.Marshal(v)
	}
	return m
}
