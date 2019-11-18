package jsonschema2go

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"io"
	"path"
	"sort"
	"text/template"
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

func (ps *Plans) Structs() (structs []structPlanContext) {
	for _, p := range ps.plans {
		if s, ok := p.(*StructPlan); ok {
			structs = append(structs, structPlanContext{ps.Imports, s})
		}
	}
	sort.Slice(structs, func(i, j int) bool {
		return structs[i].Type().Name < structs[j].Type().Name
	})
	return
}

func (ps *Plans) Arrays() (arrays []arrayPlanContext) {
	for _, p := range ps.plans {
		if a, ok := p.(*ArrayPlan); ok {
			arrays = append(arrays, arrayPlanContext{ps.Imports, a})
		}
	}
	sort.Slice(arrays, func(i, j int) bool {
		return arrays[i].Type().Name < arrays[j].Type().Name
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

type arrayPlanContext struct {
	*Imports
	*ArrayPlan
}

type enumPlanContext struct {
	*Imports
	*EnumPlan
}
