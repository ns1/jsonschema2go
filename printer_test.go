package jsonschema2go

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"go/format"
	"testing"
)

func TestImports_List(t *testing.T) {
	tests := []struct {
		name          string
		currentGoPath string
		importGoPaths []string
		wantImports   []Import
	}{
		{
			"empty",
			"github.com/jwilner/jsonschema2go",
			[]string{},
			nil,
		},
		{
			"alias",
			"github.com/jwilner/jsonschema2go",
			[]string{
				"github.com/jwilner/jsonschema2go/example",
				"github.com/jwilner/jsonschema2go/foo/example",
			},
			[]Import{
				{"github.com/jwilner/jsonschema2go/example", ""},
				{"github.com/jwilner/jsonschema2go/foo/example", "example2"},
			},
		},
		{
			"multiple",
			"github.com/jwilner/jsonschema2go",
			[]string{"encoding/json", "encoding/json"},
			[]Import{
				{"encoding/json", ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantImports, newImports(tt.currentGoPath, tt.importGoPaths).List())
		})
	}
}

func TestImports_QualName(t *testing.T) {
	tests := []struct {
		name          string
		currentGoPath string
		importGoPaths []string
		typeInfo      TypeInfo
		want          string
	}{
		{
			"builtin",
			"github.com/jwilner/jsonschema2go",
			[]string{"github.com/jwilner/jsonschema2go/example", "github.com/jwilner/jsonschema2go/foo/example"},
			TypeInfo{Name: "int"},
			"int",
		},
		{
			"external",
			"github.com/jwilner/jsonschema2go",
			[]string{"github.com/jwilner/jsonschema2go/example", "github.com/jwilner/jsonschema2go/foo/example"},
			TypeInfo{GoPath: "github.com/jwilner/jsonschema2go", Name: "Bob"},
			"Bob",
		},
		{
			"external",
			"github.com/jwilner/jsonschema2go",
			[]string{"github.com/jwilner/jsonschema2go/example", "github.com/jwilner/jsonschema2go/foo/example"},
			TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/example", Name: "Bob"},
			"example.Bob",
		},
		{
			"external with alias",
			"github.com/jwilner/jsonschema2go",
			[]string{"github.com/jwilner/jsonschema2go/example", "github.com/jwilner/jsonschema2go/foo/example"},
			TypeInfo{GoPath: "github.com/jwilner/jsonschema2go/foo/example", Name: "Bob"},
			"example2.Bob",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, newImports(tt.currentGoPath, tt.importGoPaths).QualName(tt.typeInfo))
		})
	}
}

func TestPrintFile(t *testing.T) {
	tests := []struct {
		name    string
		goPath  string
		plans   []Plan
		wantW   string
		wantErr bool
	}{
		{
			name:   "simple struct",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&StructPlan{
					Comment: "Bob does lots of cool things",
					Fields: []StructField{
						{Names: []string{"Count"}, Type: TypeInfo{Name: "int"}, Tag: `json:"count,omitempty"`},
					},
					typeInfo: TypeInfo{
						Name: "Bob",
					},
				},
			},
			wantW: `
package jsonschema2go


// Bob does lots of cool things
type Bob struct {
	Count int ` + "`" + `json:"count,omitempty"` + "`" + `
}`,
		},
		{
			name:   "struct with qualified field",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&StructPlan{
					Comment: "Bob does lots of cool things",
					Fields: []StructField{
						{Names: []string{"Count"}, Type: TypeInfo{Name: "int"}, Tag: `json:"count,omitempty"`},
						{
							Names: []string{"Other"},
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/blah",
								Name:   "OtherType",
							},
							Tag: `json:"other,omitempty"`,
						},
					},
					typeInfo: TypeInfo{
						Name: "Bob",
					},
				},
			},
			wantW: `
package jsonschema2go

import (
	"github.com/jwilner/jsonschema2go/blah"
)

// Bob does lots of cool things
type Bob struct {
	Count int 				` + "`" + `json:"count,omitempty"` + "`" + `
	Other blah.OtherType 	` + "`" + `json:"other,omitempty"` + "`" + `
}`,
		},
		{
			name:   "struct with aliased import",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&StructPlan{
					Comment: "Bob does lots of cool things",
					Fields: []StructField{
						{Names: []string{"Count"}, Type: TypeInfo{Name: "int"}, Tag: `json:"count,omitempty"`},
						{
							Names: []string{"Other"},
							Type: TypeInfo{
								GoPath:  "github.com/jwilner/jsonschema2go/blah",
								Name:    "OtherType",
								Pointer: true,
							},
							Tag: `json:"other,omitempty"`,
						},
						{
							Names: []string{"OtherOther"},
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/bob/blah",
								Name:   "AnotherType",
							},
							Tag: `json:"another,omitempty"`,
						},
					},
					typeInfo: TypeInfo{
						Name: "Bob",
					},
				},
			},
			wantW: `
package jsonschema2go

import (
	"github.com/jwilner/jsonschema2go/blah"
	blah2 "github.com/jwilner/jsonschema2go/bob/blah"
)

// Bob does lots of cool things
type Bob struct {
	Count 		int 				` + "`" + `json:"count,omitempty"` + "`" + `
	Other 		*blah.OtherType 	` + "`" + `json:"other,omitempty"` + "`" + `
	OtherOther 	blah2.AnotherType 	` + "`" + `json:"another,omitempty"` + "`" + `
}`,
		},
		{
			name:   "struct with embedded",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&StructPlan{
					Comment: "Bob does lots of cool things",
					Fields: []StructField{
						{
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go/blah",
								Name:   "OtherType",
							},
						},
					},
					typeInfo: TypeInfo{
						Name: "Bob",
					},
				},
			},
			wantW: `
package jsonschema2go

import (
	"github.com/jwilner/jsonschema2go/blah"
)

// Bob does lots of cool things
type Bob struct {
	blah.OtherType
}`,
		},
		{
			name:   "struct with embedded",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&StructPlan{
					Comment: "Bob does lots of cool things",
					Fields: []StructField{
						{
							Type: TypeInfo{
								GoPath: "github.com/jwilner/jsonschema2go",
								Name:   "OtherType",
							},
						},
					},
					typeInfo: TypeInfo{
						Name: "Bob",
					},
				},
				&StructPlan{
					Comment: "OtherType does lots of cool things",
					Fields: []StructField{
						{Type: TypeInfo{Name: "int"}, Names: []string{"Count"}, Tag: `json:"count,omitempty"`},
					},
					typeInfo: TypeInfo{
						Name: "OtherType",
					},
				},
			},
			wantW: `
package jsonschema2go

// Bob does lots of cool things
type Bob struct {
	OtherType
}

// OtherType does lots of cool things
type OtherType struct {
	Count int ` + "`" + `json:"count,omitempty"` + "`" + `
}`,
		},
		{
			name:   "array with struct",
			goPath: "github.com/jwilner/jsonschema2go",
			plans: []Plan{
				&ArrayPlan{
					typeInfo: TypeInfo{
						Name: "Bob",
					},
					Comment: "Bob does lots of cool things",
					ItemType: TypeInfo{
						GoPath: "github.com/jwilner/jsonschema2go",
						Name:   "OtherType",
					},
				},
				&StructPlan{
					Comment: "OtherType does lots of cool things",
					Fields: []StructField{
						{Type: TypeInfo{Name: "int"}, Names: []string{"Count"}, Tag: `json:"count,omitempty"`},
					},
					typeInfo: TypeInfo{
						Name: "OtherType",
					},
				},
			},
			wantW: `
package jsonschema2go

import (
	"encoding/json"
)

// OtherType does lots of cool things
type OtherType struct {
	Count int ` + "`" + `json:"count,omitempty"` + "`" + `
}

// Bob does lots of cool things
type Bob []OtherType

func (m Bob) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte(` + "`[]`" + `), nil
	}
	return json.Marshal([]OtherType(m))
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w bytes.Buffer
			err := newPrinter(nil).Print(context.Background(), &w, tt.goPath, tt.plans)
			if (err != nil) != tt.wantErr {
				t.Fatalf("printStruct() error = %v, wantErr %v", err, tt.wantErr)
			}
			formatted, err := format.Source(w.Bytes())
			if err != nil {
				t.Fatalf("unable to format: %v", err)
			}
			formattedWant, err := format.Source([]byte(tt.wantW))
			if err != nil {
				t.Fatalf("unable to format wanted: %v", err)
			}
			require.Equal(t, string(formattedWant), string(formatted))
		})
	}
}
