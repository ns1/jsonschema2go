package composite_test

import (
	"github.com/jwilner/jsonschema2go/pkg/harness"
	"testing"
)

func TestPlan(t *testing.T) {
	harness.RunGenerateTests(
		t,
		"testdata/",
		"testdata/generate",
		"github.com/jwilner/jsonschema2go/internal/composite/testdata",
	)
}

func TestValidation(t *testing.T) {
	harness.RunValidationTest(t, "testdata/validation/")
}
