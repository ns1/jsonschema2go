package validator

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/ns1/jsonschema2go/pkg/gen"
)

var SubschemaValidator = Validator{Name: "subschema", ImpliedType: "interface { Validate() error }"}

type Validator struct {
	Name                           string
	VarExpr, TestExpr, SprintfExpr *template.Template
	Deps                           []gen.TypeInfo
	ImpliedType                    string
}

func Validators(schema *gen.Schema) (styles []Validator) {
	switch typ := schema.ChooseType(); typ {
	case gen.JSONArray, gen.JSONObject:
		if !schema.Config.NoValidate && schema.AdditionalProperties == nil {
			styles = append(styles, SubschemaValidator)
		}
	case gen.JSONString:
		if schema.Pattern != nil {
			pattern := *schema.Pattern
			styles = append(styles, Validator{
				Name:        "pattern",
				VarExpr:     TemplateStr("{{ .NameSpace }}Pattern = regexp.MustCompile(`" + pattern + "`)"),
				TestExpr:    TemplateStr("!{{ .NameSpace }}Pattern.MatchString({{ .QualifiedName }})"),
				SprintfExpr: TemplateStr("`must match '" + pattern + "' but got %q`, {{ .QualifiedName }}"),
				Deps:        []gen.TypeInfo{{GoPath: "regexp", Name: "MustCompile"}},
				ImpliedType: "string",
			})
		}
		if schema.MinLength != 0 {
			lenStr := strconv.FormatUint(schema.MinLength, 10)
			styles = append(styles, Validator{
				Name:     "minLength",
				TestExpr: TemplateStr(`len({{ .QualifiedName }}) < ` + lenStr),
				SprintfExpr: TemplateStr(
					`"must have length greater than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
				ImpliedType: "string",
			})
		}
		if schema.MaxLength != nil {
			lenStr := strconv.FormatUint(*schema.MaxLength, 10)
			styles = append(styles, Validator{
				Name:     "maxLength",
				TestExpr: TemplateStr(`len({{ .QualifiedName }}) > ` + lenStr),
				SprintfExpr: TemplateStr(
					`"must have length less than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
				ImpliedType: "string",
			})
		}
	case gen.JSONInteger, gen.JSONNumber:
		impliedType := "int64"
		if typ == gen.JSONNumber {
			impliedType = "float64"
		}
		if schema.MultipleOf != nil {
			multipleOf := fmt.Sprintf("%v", *schema.MultipleOf)

			var deps []gen.TypeInfo
			expr := TemplateStr(`{{ .QualifiedName }}%` + multipleOf + ` != 0`)
			if schema.ChooseType() == gen.JSONNumber {
				deps = []gen.TypeInfo{{GoPath: "math", Name: "Mod"}}
				expr = TemplateStr(`math.Mod({{ .QualifiedName }}, ` + multipleOf + `) != 0`)
			}

			styles = append(styles, Validator{
				Name:        "multipleOf",
				TestExpr:    expr,
				SprintfExpr: TemplateStr(`"must be a multiple of ` + multipleOf + ` but was %v", {{ .QualifiedName }}`),
				Deps:        deps,
				ImpliedType: impliedType,
			})
		}
		numValidator := func(name, comparator, english string, limit float64, exclusive bool) {
			if exclusive {
				name += "Exclusive"
				comparator += "="
			} else {
				english += " or equal to"
			}
			sLimit := fmt.Sprintf("%v", limit)
			styles = append(styles, Validator{
				Name:        name,
				TestExpr:    TemplateStr(`{{ .QualifiedName }} ` + comparator + sLimit),
				SprintfExpr: TemplateStr(`"must be ` + english + ` ` + sLimit + ` but was %v", {{ .QualifiedName }}`),
				ImpliedType: impliedType,
			})
		}
		if schema.Minimum != nil {
			numValidator(
				"minimum",
				"<",
				"greater than",
				*schema.Minimum,
				schema.ExclusiveMinimum != nil && *schema.ExclusiveMinimum,
			)
		}
		if schema.Maximum != nil {
			numValidator(
				"maximum",
				">",
				"less than",
				*schema.Maximum,
				schema.ExclusiveMaximum != nil && *schema.ExclusiveMaximum,
			)
		}
	}

	if len(schema.Enum) > 0 {
		var (
			m           interface{}
			names       = make([]string, 0, len(schema.Enum))
			impliedType string
		)
		switch schema.ChooseType() {
		case gen.JSONNumber:
			mN := make(map[float64]bool, len(schema.Enum))
			for _, v := range schema.Enum {
				v, ok := v.(float64)
				if !ok {
					continue
				}
				mN[v] = true
				names = append(names, fmt.Sprintf("%v", v))
			}
			m = mN
			impliedType = "float64"
		case gen.JSONInteger:
			mI := make(map[int64]bool, len(schema.Enum))
			for _, v := range schema.Enum {
				v, ok := v.(float64)
				if !ok {
					continue
				}
				mI[int64(v)] = true
				names = append(names, fmt.Sprintf("%v", int64(v)))
			}
			m = mI
			impliedType = "int64"
		case gen.JSONString:
			mS := make(map[string]bool, len(schema.Enum))
			for _, v := range schema.Enum {
				v, ok := v.(string)
				if !ok {
					continue
				}
				mS[v] = true
				names = append(names, fmt.Sprintf("%q", v))
			}
			m = mS
			impliedType = "string"
		}
		if m != nil {
			sprintfExpr := TemplateStr("`must be one of (" + strings.Join(names, ", ") + ") but got %v`, {{ .QualifiedName }}")
			if len(names) == 1 {
				sprintfExpr = TemplateStr("`must be " + names[0] + " but got %v`, {{ .QualifiedName }}")
			}

			styles = append(styles, Validator{
				Name:        "enum",
				VarExpr:     TemplateStr("{{ .NameSpace }}Enum = " + fmt.Sprintf("%#v", m)),
				TestExpr:    TemplateStr("!{{ .NameSpace }}Enum[{{ .QualifiedName }}]"),
				SprintfExpr: sprintfExpr,
				ImpliedType: impliedType,
			})
		}
	}
	return
}

func (v *Validator) Var(nameSpace string) (string, error) {
	return tmplString(v.VarExpr, struct {
		NameSpace string
	}{nameSpace})
}

func (v *Validator) Test(nameSpace, qualifiedName string) (string, error) {
	return tmplString(v.TestExpr, struct {
		NameSpace, QualifiedName string
	}{nameSpace, qualifiedName})
}

func (v *Validator) Sprintf(nameSpace, qualifiedName string) (string, error) {
	return tmplString(v.SprintfExpr, struct {
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

func tmplString(tmpl *template.Template, v interface{}) (string, error) {
	if tmpl == nil {
		return "", nil
	}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, v)
	return string(buf.Bytes()), err
}

func TemplateStr(str string) *template.Template {
	return template.Must(template.New("").Parse(str))
}
func Sorted(vals []Validator) []Validator {
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Name < vals[j].Name
	})
	return vals
}
