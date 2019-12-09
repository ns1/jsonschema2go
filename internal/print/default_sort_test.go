package print

import (
	"github.com/jwilner/jsonschema2go/internal/composite"
	"github.com/jwilner/jsonschema2go/internal/slice"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func Test_defaultSort(t *testing.T) {
	a := [][]int{{1, 2, 3}, {2, 1, 3}, {3, 2, 1}, {2, 3, 1}, {3, 1, 2}, {1, 3, 2}}
	v := []gen.Plan{
		&composite.StructPlan{TypeInfo: gen.TypeInfo{Name: "Inner"}},
		&composite.StructPlan{TypeInfo: gen.TypeInfo{Name: "Example"}},
		&slice.Plan{TypeInfo: gen.TypeInfo{Name: "ExampleOptions"}},
	}
	for _, c := range a {
		var plans []gen.Plan
		var names []string
		for _, i := range c {
			plans = append(plans, v[i-1])
			names = append(names, v[i-1].Type().Name)
		}
		t.Run(strings.Join(names, "-"), func(t *testing.T) {
			r := require.New(t)
			res := defaultSort(plans)

			r.Equal(
				[]gen.Plan{
					&composite.StructPlan{TypeInfo: gen.TypeInfo{Name: "Example"}},
					&composite.StructPlan{TypeInfo: gen.TypeInfo{Name: "Inner"}},
					&slice.Plan{TypeInfo: gen.TypeInfo{Name: "ExampleOptions"}},
				},
				res,
			)
		})
	}
}
