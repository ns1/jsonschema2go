package print

import (
	"github.com/jwilner/jsonschema2go/internal/composite"
	"github.com/jwilner/jsonschema2go/internal/enum"
	"github.com/jwilner/jsonschema2go/internal/slice"
	"github.com/jwilner/jsonschema2go/internal/tuple"
	gen "github.com/jwilner/jsonschema2go/pkg/gen"
	"sort"
)

func defaultSort(plans []gen.Plan) []gen.Plan {
	sorted := make([]gen.Plan, len(plans))
	copy(sorted, plans)
	sort.Slice(sorted, func(i, j int) bool {
		keyI, keyJ := key(sorted[i]), key(sorted[j])
		for idx, kI := range keyI {
			kJ := keyJ[idx]
			if kI < kJ {
				return true
			}
			if kJ > kI {
				return false
			}
		}
		// equal
		return false
	})
	return sorted
}

func key(plan gen.Plan) []string {
	name := plan.Type().Name
	switch plan.(type) {
	case *composite.StructPlan:
		return []string{"a", name}
	case *slice.Plan:
		return []string{"b", name}
	case *tuple.TuplePlan:
		return []string{"c", name}
	case *enum.Plan:
		return []string{"d", name}
	default:
		return []string{"z", name}
	}
}
