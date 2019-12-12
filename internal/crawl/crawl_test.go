package crawl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/composite"
	"github.com/jwilner/jsonschema2go/internal/enum"
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/internal/validator"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"github.com/stretchr/testify/require"
	"net/url"
	"sort"
	"testing"
)

func TestSchemaToPlan(t *testing.T) {
	tests := []struct {
		name    string
		schema  *gen.Schema
		want    []gen.Plan
		wantErr bool
	}{
		{
			name: "simple",
			schema: &gen.Schema{
				Type: &gen.TypeField{gen.Object},
				Properties: map[string]*gen.RefOrSchema{
					"count": makeSchema(gen.Schema{Type: &gen.TypeField{gen.Integer}}),
				},
				Config: gen.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
			},
			want: []gen.Plan{
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []composite.StructField{
						{
							Name: "Count",
							Type: gen.TypeInfo{
								Pointer: true,
								Name:    "int64",
							},
							JSONName: "count",
							Tag:      `json:"count,omitempty"`,
						},
					},
				},
			},
		},
		{
			name: "nested struct",
			schema: &gen.Schema{
				Type: &gen.TypeField{gen.Object},
				Properties: map[string]*gen.RefOrSchema{
					"nested": makeSchema(gen.Schema{
						Config: gen.Config{
							GoPath: "github.com/jwilner/jsonschema2go/example#NestedType",
						},
						Type: &gen.TypeField{gen.Object},
						Properties: map[string]*gen.RefOrSchema{
							"count": makeSchema(gen.Schema{Type: &gen.TypeField{gen.Integer}}),
						},
					}),
				},
				Annotations: annos(map[string]string{
					"description": "i am bob",
				}),
				Config: gen.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
			},
			want: []gen.Plan{
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []composite.StructField{
						{
							Name:     "Nested",
							JSONName: "nested",
							Type: gen.TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "NestedType",
							},
							Tag:             `json:"nested,omitempty"`,
							FieldValidators: []validator.Validator{validator.SubschemaValidator},
						},
					},
				},
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "NestedType",
					},
					Fields: []composite.StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: gen.TypeInfo{
								Pointer: true,
								Name:    "int64",
							},
							Tag: `json:"count,omitempty"`,
						},
					},
				},
			},
		},
		{
			name: "composed anonymous struct",
			schema: &gen.Schema{
				Config: gen.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#AwesomeWithID",
				},
				AllOf: []*gen.RefOrSchema{
					makeSchema(
						gen.Schema{
							Type: &gen.TypeField{gen.Object},
							Properties: map[string]*gen.RefOrSchema{
								"id": makeSchema(gen.Schema{Type: &gen.TypeField{gen.Integer}}),
							},
						},
					),
					makeSchema(
						gen.Schema{
							Type: &gen.TypeField{gen.Object},
							Properties: map[string]*gen.RefOrSchema{
								"count": makeSchema(gen.Schema{Type: &gen.TypeField{gen.Integer}}),
							},
							Annotations: annos(map[string]string{
								"description": "i am bob",
							}),
							Config: gen.Config{
								GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
							},
						},
					),
				},
			},
			want: []gen.Plan{
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Comment: "i am bob",
					Fields: []composite.StructField{
						{
							Name:     "Count",
							JSONName: "count",
							Type: gen.TypeInfo{
								Pointer: true,
								Name:    "int64",
							},
							Tag: `json:"count,omitempty"`,
						},
					},
				},
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "AwesomeWithID",
					},
					Fields: []composite.StructField{
						{
							Name:     "ID",
							JSONName: "id",
							Type: gen.TypeInfo{
								Pointer: true,
								Name:    "int64",
							},
							Tag: `json:"id,omitempty"`,
						},
						{
							Type: gen.TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/example",
								Name:   "Awesome",
							},
							FieldValidators: []validator.Validator{validator.SubschemaValidator},
						},
					},
				},
			},
		},
		{
			name: "enum",
			schema: &gen.Schema{
				Config: gen.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Letter",
				},
				Type: &gen.TypeField{gen.String},
				Enum: []interface{}{
					"a",
					"b",
					"c",
				},
			},
			want: []gen.Plan{
				&enum.Plan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Letter",
					},
					BaseType: gen.TypeInfo{Name: "string"},
					Members: []enum.Member{
						{"A", "a"},
						{"B", "b"},
						{"C", "c"},
					},
				},
			},
		},
		{
			name: "nullable built in",
			schema: &gen.Schema{
				Config: gen.Config{
					GoPath: "github.com/jwilner/jsonschema2go/example#Awesome",
				},
				Type: &gen.TypeField{gen.Object},
				Properties: map[string]*gen.RefOrSchema{
					"bob": makeSchema(gen.Schema{
						OneOf: []*gen.RefOrSchema{
							makeSchema(gen.Schema{Type: &gen.TypeField{gen.Null}}),
							makeSchema(gen.Schema{Type: &gen.TypeField{gen.Integer}}),
						},
					}),
				},
			},
			want: []gen.Plan{
				&composite.StructPlan{
					TypeInfo: gen.TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go/example",
						Name:   "Awesome",
					},
					Fields: []composite.StructField{
						{
							Name:     "Bob",
							JSONName: "bob",
							Type:     gen.TypeInfo{Name: "int64", Pointer: true},
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
				got []gen.Plan
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
			// not gonna deal w/ traits atm
			for _, g := range got {
				if g, ok := g.(*composite.StructPlan); ok {
					g.Traits = nil
				}
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func schemaChan(schemas ...*gen.Schema) <-chan *gen.Schema {
	schemaC := make(chan *gen.Schema)
	go func() {
		for _, s := range schemas {
			schemaC <- s
		}
		close(schemaC)
	}()
	return schemaC
}

func annos(annos map[string]string) gen.TagMap {
	m := make(map[string]json.RawMessage)
	for k, v := range annos {
		m[k], _ = json.Marshal(v)
	}
	return m
}

func makeSchema(s gen.Schema) *gen.RefOrSchema {
	return gen.NewRefOrSchema(&s, nil)
}

type mockLoader map[string]*gen.Schema

func (m mockLoader) Load(ctx context.Context, u *url.URL) (*gen.Schema, error) {
	if u.Scheme != "file" || u.Host != "" {
		return nil, fmt.Errorf("expected \"file\" scheme and no host but got %q and %q: %q", u.Scheme, u.Host, u)
	}
	v, ok := m[u.Path]
	if !ok {
		return nil, fmt.Errorf("%q not found", u)
	}
	return v, nil
}

func (m mockLoader) Close() error {
	return nil
}
