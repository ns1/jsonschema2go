package jsonschema2go

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"go/format"
	"testing"
)

func Test_printStruct(t *testing.T) {
	tests := []struct {
		name    string
		plan    *StructPlan
		wantW   string
		wantErr bool
	}{
		{
			name: "simple struct",
			plan: &StructPlan{
				Comment: "Bob does lots of cool things",
				Fields: []StructField{
					{Names: []string{"Count"}, Type: BuiltInInt, Tag: `json="count,omitempty"`},
				},
				typeInfo: TypeInfo{
					Name: "Bob",
				},
			},
			wantW: `
// Bob does lots of cool things
type Bob struct {
	Count int ` + "`" + `json="count,omitempty"` + "`" + `  
}`,
		},
		{
			name: "struct with qualified field",
			plan: &StructPlan{
				Comment: "Bob does lots of cool things",
				Fields: []StructField{
					{Names: []string{"Count"}, Type: BuiltInInt, Tag: `json="count,omitempty"`},
					{
						Names: []string{"Other"},
						Type: TypeInfo{
							GoPath: "github.com/jwilner/jsonschema2go/blah",
							Name:   "OtherType",
						},
						Tag: `json="other,omitempty"`,
					},
				},
				typeInfo: TypeInfo{
					Name: "Bob",
				},
			},
			wantW: `
// Bob does lots of cool things
type Bob struct {
	Count int 				` + "`" + `json="count,omitempty"` + "`" + `  
	Other blah.OtherType 	` + "`" + `json="other,omitempty"` + "`" + `  
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w bytes.Buffer
			err := printStruct(context.Background(), &w, tt.plan)
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
