package generate

import (
	"fmt"
	"path"
	"sort"
)

type Import struct {
	GoPath, Alias string
}

type Imports struct {
	currentGoPath string
	aliases       map[string]string
}

func NewImports(currentGoPath string, importGoPaths []string) *Imports {
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

type TypeInfo struct {
	GoPath  string
	Name    string
	Pointer bool
	ValPath string
}

func (t TypeInfo) BuiltIn() bool {
	return t.GoPath == ""
}

func (t TypeInfo) Unknown() bool {
	return t == TypeInfo{}
}
