package jsonschema2go

import (
	"bytes"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

type TypeInfo struct {
	GoPath  string
	Name    string
	Pointer bool
	valPath string
}

func (t TypeInfo) ValPath() string {
	if t.valPath != "" {
		return "." + t.valPath
	}
	return ""
}

func (t TypeInfo) BuiltIn() bool {
	return t.GoPath == ""
}

func (t TypeInfo) Unknown() bool {
	return t == TypeInfo{}
}

var primitives = map[SimpleType]string{
	Boolean: "bool",
	Integer: "int64",
	Number:  "float64",
	Null:    "interface{}",
	String:  "string",
}

type TuplePlan struct {
	typeInfo TypeInfo
	id       *url.URL
	Comment  string

	Items []*TupleItem
}

func (t *TuplePlan) ArrayLength() int {
	return len(t.Items)
}

func (t *TuplePlan) Type() TypeInfo {
	return t.typeInfo
}

func (t *TuplePlan) Deps() []TypeInfo {
	deps := []TypeInfo{
		{GoPath: "encoding/json", Name: "Marshal"},
		{GoPath: "encoding/json", Name: "Unmarshal"},
		{GoPath: "fmt", Name: "Sprintf"},
	}
	for _, f := range t.Items {
		deps = append(deps, f.Type)
		for _, v := range f.validators {
			deps = append(deps, v.Deps...)
		}
	}
	return deps
}

func (t *TuplePlan) ID() string {
	if t.id != nil {
		return t.id.String()
	}
	return ""
}

func (s *TuplePlan) ValidateInitialize() bool {
	for _, f := range s.Items {
		for _, v := range f.validators {
			if v.varExpr != nil {
				return true
			}
		}
	}
	return false
}

type TupleItem struct {
	Comment    string
	Type       TypeInfo
	validators []Validator
}

func (t TupleItem) Validators() []Validator {
	return sortedValidators(t.validators)
}

func sortedValidators(vals []Validator) []Validator {
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Name < vals[j].Name
	})
	return vals
}

type StructField struct {
	Comment    string
	Name       string
	JSONName   string
	Type       TypeInfo
	Tag        string
	Required   bool
	validators []Validator
}

func (s StructField) Validators() []Validator {
	return sortedValidators(s.validators)
}

type Validator struct {
	Name                           string
	varExpr, testExpr, sprintfExpr *template.Template
	Deps                           []TypeInfo
	ImpliedType                    string
}

func tmplString(tmpl *template.Template, v interface{}) (string, error) {
	if tmpl == nil {
		return "", nil
	}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, v)
	return string(buf.Bytes()), err
}

func (v *Validator) VarExpr(nameSpace string) (string, error) {
	return tmplString(v.varExpr, struct {
		NameSpace string
	}{nameSpace})
}

func (v *Validator) TestExpr(nameSpace, qualifiedName string) (string, error) {
	return tmplString(v.testExpr, struct {
		NameSpace, QualifiedName string
	}{nameSpace, qualifiedName})
}

func (v *Validator) Sprintf(nameSpace, qualifiedName string) (string, error) {
	return tmplString(v.sprintfExpr, struct {
		NameSpace, QualifiedName string
	}{nameSpace, qualifiedName})
}

func (Validator) NameSpace(names ...interface{}) string {
	strs := make([]string, 0, len(names))
	for _, n := range names {
		strs = append(strs, fmt.Sprint(n))
	}

	name := strings.Join(strs, "")
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

type Trait interface {
	Template() string
}

type Plan interface {
	Type() TypeInfo
	Deps() []TypeInfo
	ID() string
}

type boxedEncodingTrait struct{}

func (boxedEncodingTrait) Template() string {
	return "boxed"
}

func (boxedEncodingTrait) Deps() []TypeInfo {
	return []TypeInfo{{GoPath: "encoding/json", Name: "Marshal"}}
}

type StructPlan struct {
	typeInfo TypeInfo
	id       *url.URL

	Comment string
	Fields  []StructField
	Traits  []Trait
}

func (s *StructPlan) Type() TypeInfo {
	return s.typeInfo
}

func (s *StructPlan) ValidateInitialize() bool {
	for _, f := range s.Fields {
		for _, v := range f.validators {
			if v.varExpr != nil {
				return true
			}
		}
	}
	return false
}

func (s *StructPlan) ID() string {
	if s.id != nil {
		return s.id.String()
	}
	return ""
}

func (s *StructPlan) Deps() (deps []TypeInfo) {
	deps = append(deps, TypeInfo{Name: "Sprintf", GoPath: "fmt"})
	for _, f := range s.Fields {
		deps = append(deps, f.Type)
		for _, v := range f.validators {
			deps = append(deps, v.Deps...)
		}
	}
	for _, t := range s.Traits {
		if t, ok := t.(interface{ Deps() []TypeInfo }); ok {
			deps = append(deps, t.Deps()...)
		}
	}
	return
}

type SlicePlan struct {
	typeInfo TypeInfo
	id       *url.URL

	Comment        string
	ItemType       TypeInfo
	validators     []Validator
	itemValidators []Validator
}

func (a *SlicePlan) ID() string {
	if a.id != nil {
		return a.id.String()
	}
	return ""
}

func (a *SlicePlan) Type() TypeInfo {
	return a.typeInfo
}

func (a *SlicePlan) Deps() []TypeInfo {
	return []TypeInfo{a.ItemType, {Name: "Marshal", GoPath: "encoding/json"}, {Name: "Sprintf", GoPath: "fmt"}}
}

func (a *SlicePlan) Validators() []Validator {
	sort.Slice(a.validators, func(i, j int) bool {
		return a.validators[i].Name < a.validators[j].Name
	})
	return a.validators
}

func (a *SlicePlan) ItemValidators() []Validator {
	sort.Slice(a.itemValidators, func(i, j int) bool {
		return a.itemValidators[i].Name < a.itemValidators[j].Name
	})
	return a.itemValidators
}

func (a *SlicePlan) ItemValidateInitialize() bool {
	for _, i := range a.itemValidators {
		if i.varExpr != nil {
			return true
		}
	}
	return false
}

type EnumPlan struct {
	typeInfo TypeInfo
	id       *url.URL

	Comment  string
	BaseType TypeInfo
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

func (e *EnumPlan) Type() TypeInfo {
	return e.typeInfo
}

func (e *EnumPlan) Deps() []TypeInfo {
	return []TypeInfo{e.BaseType, {Name: "Sprintf", GoPath: "fmt"}}
}
