package jsonschema2go

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"io"
	"path"
	"sort"
	"text/template"
	"unicode"
)

//go:generate go run internal/cmd/gentmpl/gentmpl.go

type Import struct {
	GoPath, Alias string
}

type Imports struct {
	currentGoPath string
	aliases       map[string]string
}

func newImports(currentGoPath string, importGoPaths []string) *Imports {
	baseName := make(map[string]map[string]bool)
	for _, i := range importGoPaths {
		if i != "" && i != currentGoPath {
			pkg := path.Base(i)
			if _, ok := baseName[pkg]; !ok {
				baseName[pkg] = make(map[string]bool)
			}
			baseName[pkg][i] = true
		}
	}

	aliases := make(map[string]string)
	for k, v := range baseName {
		if len(v) == 1 {
			for i := range v {
				aliases[i] = ""
			}
			continue
		}
		imps := make([]string, 0, len(v))
		for i := range v {
			imps = append(imps, i)
		}
		sort.Strings(imps)

		for i, path := range imps {
			if i == 0 {
				aliases[path] = ""
				continue
			}
			aliases[path] = fmt.Sprintf("%s%d", k, i+1)
		}
	}

	return &Imports{currentGoPath, aliases}
}

func (i *Imports) CurPackage() string {
	return path.Base(i.currentGoPath)
}

func (i *Imports) List() (imports []Import) {
	for path, alias := range i.aliases {
		imports = append(imports, Import{path, alias})
	}
	sort.Slice(imports, func(i, j int) bool {
		return imports[i].GoPath < imports[j].GoPath
	})
	return
}

func (i *Imports) QualName(info TypeInfo) string {
	if info.BuiltIn() || info.GoPath == i.currentGoPath {
		return info.Name
	}
	qual := path.Base(info.GoPath)
	if alias := i.aliases[info.GoPath]; alias != "" {
		qual = alias
	}
	return fmt.Sprintf("%s.%s", qual, info.Name)
}

func newPrinter(tmpl *template.Template) *Printer {
	if tmpl == nil {
		tmpl = valueTmpl
	}
	return &Printer{valueTmpl}
}

type Printer struct {
	tmpl *template.Template
}

func (p *Printer) Print(ctx context.Context, w io.Writer, goPath string, plans []Plan) error {
	var imports *Imports
	{
		var depPaths []string
		for _, pl := range plans {
			for _, d := range pl.Deps() {
				depPaths = append(depPaths, d.GoPath)
			}
		}
		imports = newImports(goPath, depPaths)
	}

	var buf bytes.Buffer
	if err := valueTmpl.Execute(&buf, &Plans{imports, plans}); err != nil {
		return fmt.Errorf("unable to execute tmpl: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		_, _ = w.Write(buf.Bytes()) // write unformatted for debugging
		return fmt.Errorf("unable to format: %w", err)
	}

	_, err = w.Write(formatted)
	return err
}

type Plans struct {
	Imports *Imports
	plans   []Plan
}

func (ps *Plans) Structs() (structs []*structPlanContext) {
	for _, p := range ps.plans {
		if s, ok := p.(*StructPlan); ok {
			structs = append(structs, &structPlanContext{ps.Imports, s})
		}
	}
	sort.Slice(structs, func(i, j int) bool {
		return structs[i].Type().Name < structs[j].Type().Name
	})
	return
}

func (ps *Plans) Slices() (slices []*slicePlanContext) {
	for _, p := range ps.plans {
		if a, ok := p.(*SlicePlan); ok {
			slices = append(slices, &slicePlanContext{ps.Imports, a})
		}
	}
	sort.Slice(slices, func(i, j int) bool {
		return slices[i].Type().Name < slices[j].Type().Name
	})
	return
}

func (ps *Plans) Tuples() (tuples []*tuplePlanContext) {
	for _, p := range ps.plans {
		if a, ok := p.(*TuplePlan); ok {
			tuples = append(tuples, &tuplePlanContext{ps.Imports, a})
		}
	}
	sort.Slice(tuples, func(i, j int) bool {
		return tuples[i].Type().Name < tuples[j].Type().Name
	})
	return
}

func (ps *Plans) Enums() (enums []enumPlanContext) {
	for _, p := range ps.plans {
		if e, ok := p.(*EnumPlan); ok {
			enums = append(enums, enumPlanContext{ps.Imports, e})
		}
	}
	sort.Slice(enums, func(i, j int) bool {
		return enums[i].Type().Name < enums[j].Type().Name
	})
	return
}

type structPlanContext struct {
	*Imports
	*StructPlan
}

type EnrichedStructField struct {
	StructField
	StructPlan *StructPlan
	Imports    *Imports
}

func (s *structPlanContext) Fields() (fields []EnrichedStructField) {
	for _, f := range s.StructPlan.Fields {
		fields = append(fields, EnrichedStructField{
			StructField: f,
			StructPlan:  s.StructPlan,
			Imports:     s.Imports,
		})
	}
	return
}

func (f *EnrichedStructField) DerefExpr() string {
	valPath := f.Type.valPath
	if valPath != "" {
		valPath = "." + valPath
	}
	return fmt.Sprintf("m.%s%s", f.Name, valPath)
}

func (f *EnrichedStructField) TestSetExpr(pos bool) (string, error) {
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
		op := ""
		if !pos {
			op = "!"
		}
		return fmt.Sprintf("%sm.%s.Set", op, f.Name), nil
	}
	if f.Type.Name == "interface{}" || f.Type.Pointer {
		op := "!="
		if !pos {
			op = "=="
		}
		return fmt.Sprintf("m.%s %s nil", f.Name, op), nil
	}
	return "", errors.New("no test set expr")
}

func (f *EnrichedStructField) NameSpace() string {
	name := fmt.Sprintf("%s%s", f.StructPlan.Type().Name, f.Name)
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

func (f *EnrichedStructField) FieldDecl() string {
	typ := f.Imports.QualName(f.Type)
	if f.Type.Pointer {
		typ = "*" + typ
	}
	tag := f.Tag
	if tag != "" {
		tag = "`" + tag + "`"
	}
	return f.Name + " " + typ + tag
}

func (f *EnrichedStructField) InnerFieldDecl() string {
	typName := f.Imports.QualName(f.Type)
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
		s := []rune(f.Type.Name)
		s[0] = unicode.ToLower(s[0])

		typName = "*" + string(s)
	}
	tag := ""
	if f.Name != "" { // not an embedded struct
		tag = fmt.Sprintf("`json:"+`"%s,omitempty"`+"`", f.JSONName)
	}
	return fmt.Sprintf("%s %s %s", f.Name, typName, tag)
}

func (f *EnrichedStructField) Embedded() bool {
	return f.Name == ""
}

func (f *EnrichedStructField) FieldRef() string {
	if f.Name != "" {
		return f.Name
	}
	return f.Type.Name // embedded
}

func (f *EnrichedStructField) InnerFieldLiteral() string {
	if f.Type.GoPath == "github.com/jwilner/jsonschema2go/boxed" {
		return ""
	}
	fieldRef := f.Name
	if fieldRef == "" { // embedded
		fieldRef = f.Type.Name
	}
	return fmt.Sprintf("%s: m.%s,", fieldRef, fieldRef)
}

var fieldAssignmentTmpl = templateStr(`if m.{{ .Name }}.Set {
	inner.{{ .Name }} = &m.{{ .Name }}{{ .ValPath }}
}`)

func (f *EnrichedStructField) InnerFieldAssignment() (string, error) {
	if f.Type.GoPath != "github.com/jwilner/jsonschema2go/boxed" {
		return "", nil
	}

	var w bytes.Buffer
	err := fieldAssignmentTmpl.Execute(&w, struct {
		Name    string
		ValPath string
	}{
		Name:    f.Name,
		ValPath: f.Type.ValPath(),
	})
	return w.String(), err
}

type slicePlanContext struct {
	*Imports
	*SlicePlan
}

type enumPlanContext struct {
	*Imports
	*EnumPlan
}

type tuplePlanContext struct {
	*Imports
	*TuplePlan
}

type enrichedTupleItem struct {
	TuplePlan *tuplePlanContext
	idx       int
	*TupleItem
}

func (e *enrichedTupleItem) NameSpace() string {
	name := fmt.Sprintf("%s%d", e.TuplePlan.Type().Name, e.idx)
	if len(name) > 0 {
		runes := []rune(name)
		runes[0] = unicode.ToLower(runes[0])
		name = string(runes)
	}
	return name
}

func (t *tuplePlanContext) Items() (items []*enrichedTupleItem) {
	for idx, item := range t.TuplePlan.Items {
		items = append(items, &enrichedTupleItem{t, idx, item})
	}
	return
}
