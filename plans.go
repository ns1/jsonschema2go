package jsonschema2go

import (
	"bytes"
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
	sort.Slice(s.validators, func(i, j int) bool {
		return s.validators[i].Name < s.validators[j].Name
	})
	return s.validators
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

func (Validator) NameSpace(names ...string) string {
	name := strings.Join(names, "")
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
	return "boxed.tmpl"
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

type ArrayPlan struct {
	typeInfo TypeInfo
	id       *url.URL

	Comment        string
	ItemType       TypeInfo
	validators     []Validator
	itemValidators []Validator
}

func (a *ArrayPlan) ID() string {
	if a.id != nil {
		return a.id.String()
	}
	return ""
}

func (a *ArrayPlan) Type() TypeInfo {
	return a.typeInfo
}

func (a *ArrayPlan) Deps() []TypeInfo {
	return []TypeInfo{a.ItemType, {Name: "Marshal", GoPath: "encoding/json"}, {Name: "Sprintf", GoPath: "fmt"}}
}

func (a *ArrayPlan) Validators() []Validator {
	sort.Slice(a.validators, func(i, j int) bool {
		return a.validators[i].Name < a.validators[j].Name
	})
	return a.validators
}

func (a *ArrayPlan) ItemValidators() []Validator {
	sort.Slice(a.itemValidators, func(i, j int) bool {
		return a.itemValidators[i].Name < a.itemValidators[j].Name
	})
	return a.itemValidators
}

func (a *ArrayPlan) ItemValidateInitialize() bool {
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
