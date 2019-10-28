package jsonschema2go

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"text/template"
)

var (
	structTmpl = template.Must(fileTmplWithFuncs("templates/struct.tmpl"))
)

type Imports struct {
	currentGoPath string
	aliases       map[string]string
}

func newImports(currentGoPath string, importGoPaths []string) *Imports {
	baseName := make(map[string][]string)
	for _, i := range importGoPaths {
		if i != "" && i != currentGoPath {
			pkg := path.Base(i)
			baseName[pkg] = append(baseName[pkg], i)
		}
	}

	aliases := make(map[string]string)
	for k, v := range baseName {
		if len(v) == 1 {
			aliases[v[0]] = ""
			continue
		}
		sort.Strings(v)

		for i, path := range v {
			if i == 0 {
				aliases[path] = ""
				continue
			}
			aliases[path] = fmt.Sprintf("%s%d", k, i+1)
		}
	}

	return &Imports{currentGoPath, aliases}
}

func (i *Imports) QualName(info TypeInfo) string {
	if info.BuiltIn() || info.GoPath == i.currentGoPath {
		return info.Name
	}
	qual := path.Base(info.GoPath)
	if alias, ok := i.aliases[info.GoPath]; ok {
		qual = alias
	}
	return fmt.Sprintf("%s.%s", qual, info.Name)
}

func fileTmplWithFuncs(fName string) (*template.Template, error) {
	return template.New(filepath.Base(fName)).ParseFiles(fName)
}

type planner interface {
	Type() TypeInfo
	Deps() []TypeInfo
}

func PrintFile(ctx context.Context, w io.Writer, plans []planner) error {
	return nil
}

type structPlanContext struct {
	*Imports
	*StructPlan
}

func printStruct(_ context.Context, w io.Writer, plan *StructPlan) error {
	return structTmpl.Execute(w, structPlanContext{new(Imports), plan})
}