package planning

import (
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"net/url"
	"sort"
)

type Trait interface {
	Template() string
}

type BoxedEncodingTrait struct{}

func (BoxedEncodingTrait) Template() string {
	return "boxed"
}

func (BoxedEncodingTrait) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{{GoPath: "encoding/json", Name: "Marshal"}}
}

func sortedValidators(vals []Validator) []Validator {
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Name < vals[j].Name
	})
	return vals
}

type EnumPlan struct {
	TypeInfo generate.TypeInfo
	id       *url.URL

	Comment  string
	BaseType generate.TypeInfo
	Members  []EnumMember
}

func (e *EnumPlan) ID() string {
	if e.id != nil {
		return e.id.String()
	}
	return ""
}

type EnumMember struct {
	Name  string
	Field interface{}
}

func (e *EnumPlan) Type() generate.TypeInfo {
	return e.TypeInfo
}

func (e *EnumPlan) Deps() []generate.TypeInfo {
	return []generate.TypeInfo{e.BaseType, {Name: "Sprintf", GoPath: "fmt"}}
}

func (e *EnumPlan) Printable(imp *generate.Imports) generate.PrintablePlan {
	return &enumPlanContext{Imports: imp, EnumPlan: e}
}