package jsonschema2go

import "testing"

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
			if got := defaultTyper.JSONPropertyExported(tt.input); got != tt.want {
				t.Errorf("jsonPropertyToExportedName() = %v, want %v", got, tt.want)
			}
		})
	}
}
