package print

import (
	"github.com/jwilner/jsonschema2go/internal/planning"
	gen "github.com/jwilner/jsonschema2go/pkg/generate"
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
		return false
	})
	return sorted
}

func key(plan gen.Plan) []string {
	name := plan.Type().Name
	switch plan.(type) {
	case *planning.StructPlan:
		return []string{"a", name}
	case *planning.SlicePlan:
		return []string{"b", name}
	case *planning.TuplePlan:
		return []string{"c", name}
	case *planning.EnumPlan:
		return []string{"d", name}
	default:
		return []string{"z", name}
	}
}
