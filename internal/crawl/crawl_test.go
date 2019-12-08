package crawl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"github.com/jwilner/jsonschema2go/pkg/schema"
	"github.com/stretchr/testify/require"
	"net/url"
	"sort"
	"testing"
)

func TestSchemaToPlan(t *testing.T) {
	tests := []struct {
		name    string
		schema  *schema.Schema
		want    []generate.Plan
		wantErr bool
	}{
		{
			name: "simple",
			schema: &schema.Schema{
				Type: &schema.TypeField{schema.Object},
				Properties: map[string]*schema.RefOrSchema{
					"count": makeSchema(schema.Schema{Type: &schema.TypeField{schema.Integer}}),
				},
				Config: schema.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
			},
			want: []generate.Plan{
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []planning.StructField{
						{
							Name: "Count",
							Type: generate.TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								ValPath: "Int64",
							},
							JSONName: "count",
							Tag:      `json:"count"`,
						},
					},
					Traits: []planning.Trait{&planning.BoxedEncodingTrait{}},
				},
			},
		},
		{
			name: "nested struct",
			schema: &schema.Schema{
				Type: &schema.TypeField{schema.Object},
				Properties: map[string]*schema.RefOrSchema{
					"nested": makeSchema(schema.Schema{
						Config: schema.Config{
							GoPath: "github.com/jwilner/jsonschema2go/example#NestedType",
						},
						Type: &schema.TypeField{schema.Object},
						Properties: map[string]*schema.RefOrSchema{
							"count": makeSchema(schema.Schema{Type: &schema.TypeField{schema.Integer}}),
						},
					}),
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
				Config: schema.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
			},
			want: []generate.Plan{
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []planning.StructField{
						{
							Name:     "Nested",
							JSONName: "nested",
							Type: generate.TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "NestedType",
							},
							Tag:             `json:"nested,omitempty"`,
							FieldValidators: []planning.Validator{planning.SubschemaValidator},
						},
					},
				},
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "NestedType",
					},
					Fields: []planning.StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: generate.TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								ValPath: "Int64",
							},
							Tag: `json:"count"`,
						},
					},
					Traits: []planning.Trait{&planning.BoxedEncodingTrait{}},
				},
			},
		},
		{
			name: "composed anonymous struct",
			schema: &schema.Schema{
				Config: schema.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#AwesomeWithID",
				},
				AllOf: []*schema.RefOrSchema{
					makeSchema(
						schema.Schema{
							Type: &schema.TypeField{schema.Object},
							Properties: map[string]*schema.RefOrSchema{
								"id": makeSchema(schema.Schema{Type: &schema.TypeField{schema.Integer}}),
							},
						},
					),
					makeSchema(
						schema.Schema{
							Type: &schema.TypeField{schema.Object},
							Properties: map[string]*schema.RefOrSchema{
								"count": makeSchema(schema.Schema{Type: &schema.TypeField{schema.Integer}}),
							},
							Annotations: annos(map[string]string{
								"description": "i am bob",
							}),
							Config: schema.Config{
								GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
							},
						},
					),
				},
			},
			want: []generate.Plan{
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []planning.StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: generate.TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								ValPath: "Int64",
							},
							Tag: `json:"count"`,
						},
					},
					Traits: []planning.Trait{&planning.BoxedEncodingTrait{}},
				},
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "AwesomeWithID",
					},
					Fields: []planning.StructField{
						{
							Name:     "ID",
							JSONName: "id",
							Type: generate.TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/boxed",
								Name:    "Int64",
								ValPath: "Int64",
							},
							Tag: `json:"id"`,
						},
						{
							Type: generate.TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "Awesome",
							},
							FieldValidators: []planning.Validator{planning.SubschemaValidator},
						},
					},
					Traits: []planning.Trait{&planning.BoxedEncodingTrait{}},
				},
			},
		},
		{
			name: "enum",
			schema: &schema.Schema{
				Config: schema.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Letter",
				},
				Type: &schema.TypeField{schema.String},
				Enum: []interface{}{
					"a",
					"b",
					"c",
				},
			},
			want: []generate.Plan{
				&planning.EnumPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Letter",
					},
					BaseType: generate.TypeInfo{Name: "string"},
					Members: []planning.EnumMember{
						{"A", "a"},
						{"B", "b"},
						{"C", "c"},
					},
				},
			},
		},
		{
			name: "nullable built in",
			schema: &schema.Schema{
				Config: schema.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Type: &schema.TypeField{schema.Object},
				Properties: map[string]*schema.RefOrSchema{
					"bob": makeSchema(schema.Schema{
						OneOf: []*schema.RefOrSchema{
							makeSchema(schema.Schema{Type: &schema.TypeField{schema.Null}}),
							makeSchema(schema.Schema{Type: &schema.TypeField{schema.Integer}}),
						},
					}),
				},
			},
			want: []generate.Plan{
				&planning.StructPlan{
					TypeInfo: generate.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Fields: []planning.StructField{
						{
							Name:     "Bob",
							JSONName: "bob",
							Type:     generate.TypeInfo{Name: "int64", Pointer: true},
							Tag:      `json:"bob,omitempty"`,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := crawl(context.Background(), planning.Composite, mockLoader{}, planning.DefaultTyper, schemaChan(tt.schema))
			var (
				got []generate.Plan
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

func schemaChan(schemas ...*schema.Schema) <-chan *schema.Schema {
	schemaC := make(chan *schema.Schema)
	go func() {
		for _, s := range schemas {
			schemaC <- s
		}
		close(schemaC)
	}()
	return schemaC
}

func annos(annos map[string]string) schema.TagMap {
	m := make(map[string]json.RawMessage)
	for k, v := range annos {
		m[k], _ = json.Marshal(v)
	}
	return m
}

func makeSchema(s schema.Schema) *schema.RefOrSchema {
	return schema.NewRefOrSchema(&s, nil)
}

type mockLoader map[string]*schema.Schema

func (m mockLoader) Load(ctx context.Context, u *url.URL) (*schema.Schema, error) {
	if u.Scheme != "file" || u.Host != "" {
		return nil, fmt.Errorf("expected \"file\" scheme and no host but got %q and %q: %q", u.Scheme, u.Host, u)
	}
	v, ok := m[u.Path]
	if !ok {
		return nil, fmt.Errorf("%q not found", u)
	}
	return v, nil
}
