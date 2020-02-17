package planning

import (
	"testing"
)

func Test_jsonPropertyToExportedName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "snake case",
			input: "hi_how_are_you",
			want:  "HiHowAreYou",
		},
		{
			name:  "dashed snake case",
			input: "hi-how-are-you",
			want:  "HiHowAreYou",
		},
		{
			name:  "spaces",
			input: "hi how are you",
			want:  "HiHowAreYou",
		},
		{
			name:  "camel case",
			input: "hiHowAreYou",
			want:  "HiHowAreYou",
		},
		{
			name:  "all lower",
			input: "hello",
			want:  "Hello",
		},
		{
			name:  "weird initialism in json",
			input: "HTTP",
			want:  "HTTP",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultTyper.JSONPropertyExported(tt.input); got != tt.want {
				t.Errorf("jsonPropertyToExportedName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_typeFromID(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		pairs                  [][2]string
		id, wantPath, wantName string
	}{
		{
			name:     "maps",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps no extension",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps no pairs",
			pairs:    [][2]string{},
			id:       "https://example.com/v1/blah/bar",
			wantPath: "example.com/v1/blah",
			wantName: "bar",
		},
		{
			name:     "maps no scheme",
			pairs:    [][2]string{},
			id:       "example.com/v1/blah/bar",
			wantPath: "example.com/v1/blah",
			wantName: "bar",
		},
		{
			name:     "maps empty fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#",
			wantPath: "github.com/example/blah",
			wantName: "bar",
		},
		{
			name:     "maps properties fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#/properties/baz",
			wantPath: "github.com/example/blah",
			wantName: "barBaz",
		},
		{
			name:     "maps extended fragment",
			pairs:    [][2]string{{"https://example.com/v1/", "github.com/example/"}},
			id:       "https://example.com/v1/blah/bar.json#/properties/baz/items/2/properties/hello",
			wantPath: "github.com/example/blah",
			wantName: "barBazItems2Hello",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if path, name := TypeFromId(tt.pairs)(tt.id); tt.wantName != name || tt.wantPath != path {
				t.Errorf("wanted (%q, %q) got (%q, %q)", tt.wantPath, tt.wantName, path, name)
			}
		})
	}
}

func Test_mapPrefix(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefixes [][2]string
		want     string
	}{
		{"empty", "blah", nil, "blah"},
		{"one", "github.com/jsonschema2go/foo/bar", [][2]string{{"github.com/jsonschema2go", "code"}}, "code/foo/bar"},
		{
			"greater",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/jsonschema2go", "code"}, {"github.com/otherpath", "blob"}},
			"code/foo/bar",
		},
		{
			"less",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/a", "other"}, {"github.com/jsonschema2go", "code"}},
			"code/foo/bar",
		},
		{
			"takes longest",
			"github.com/jsonschema2go/foo/bar",
			[][2]string{{"github.com/", "other"}, {"github.com/jsonschema2go", "code"}},
			"code/foo/bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrefixMapper(tt.prefixes)(tt.path); got != tt.want {
				t.Errorf("mapPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
