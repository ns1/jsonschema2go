package tuple_test

import (
	"github.com/jwilner/jsonschema2go/pkg/testharness"
	"testing"
)

func TestPlan(t *testing.T) {
	testharness.RunGenerateTests(
		t,
		"testdata/",
		"testdata/generate",
		"github.com/jwilner/jsonschema2go/internal/tuple/testdata",
	)
}
