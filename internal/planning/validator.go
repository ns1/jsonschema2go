package planning

import (
	"bytes"
	"fmt"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

var SubschemaValidator = Validator{Name: "subschema", ImpliedType: "interface { Validate() error }"}

type Validator struct {
	Name                           string
	varExpr, testExpr, sprintfExpr *template.Template
	Deps                           []generate.TypeInfo
	ImpliedType                    string
}

func validators(schema *sch.Schema) (styles []Validator) {
	switch typ := schema.ChooseType(); typ {
	case sch.Array, sch.Object:
		if !schema.Config.NoValidate {
			styles = append(styles, SubschemaValidator)
		}
	case sch.String:
		if schema.Pattern != nil {
			pattern := *schema.Pattern
			styles = append(styles, Validator{
				Name:        "pattern",
				varExpr:     TemplateStr("{{ .NameSpace }}Pattern = regexp.MustCompile(`" + pattern + "`)"),
				testExpr:    TemplateStr("!{{ .NameSpace }}Pattern.MatchString({{ .QualifiedName }})"),
				sprintfExpr: TemplateStr(`"must match '` + pattern + `' but got %q", {{ .QualifiedName }}`),
				Deps:        []generate.TypeInfo{{GoPath: "regexp", Name: "MustCompile"}},
				ImpliedType: "string",
			})
		}
		if schema.MinLength != 0 {
			lenStr := strconv.FormatUint(schema.MinLength, 10)
			styles = append(styles, Validator{
				Name:     "minLength",
				testExpr: TemplateStr(`len({{ .QualifiedName }}) < ` + lenStr),
				sprintfExpr: TemplateStr(
					`"must have length greater than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
				ImpliedType: "string",
			})
		}
		if schema.MaxLength != nil {
			lenStr := strconv.FormatUint(*schema.MaxLength, 10)
			styles = append(styles, Validator{
				Name:     "maxLength",
				testExpr: TemplateStr(`len({{ .QualifiedName }}) > ` + lenStr),
				sprintfExpr: TemplateStr(
					`"must have length less than ` + lenStr + ` but was %d", len({{ .QualifiedName }})`,
				),
				ImpliedType: "string",
			})
		}
	case sch.Integer, sch.Number:
		impliedType := "int64"
		if typ == sch.Number {
			impliedType = "float64"
		}
		if schema.MultipleOf != nil {
			multipleOf := fmt.Sprintf("%v", *schema.MultipleOf)

			var deps []generate.TypeInfo
			expr := TemplateStr(`{{ .QualifiedName }}%` + multipleOf + ` != 0`)
			if schema.ChooseType() == sch.Number {
				deps = []generate.TypeInfo{{GoPath: "math", Name: "Mod"}}
				expr = TemplateStr(`math.Mod({{ .QualifiedName }}, ` + multipleOf + `) != 0`)
			}

			styles = append(styles, Validator{
				Name:        "multipleOf",
				testExpr:    expr,
				sprintfExpr: TemplateStr(`"must be a multiple of ` + multipleOf + ` but was %v", {{ .QualifiedName }}`),
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
				testExpr:    TemplateStr(`{{ .QualifiedName }} ` + comparator + sLimit),
				sprintfExpr: TemplateStr(`"must be ` + english + ` ` + sLimit + ` but was %v", {{ .QualifiedName }}`),
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
				*schema.Minimum,
				schema.ExclusiveMinimum != nil && *schema.ExclusiveMinimum,
			)
		}
	}
	return
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
