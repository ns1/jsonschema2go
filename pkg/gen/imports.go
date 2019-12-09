package gen

import (
	"fmt"
	"path"
	"sort"
)

// Imports encapsulates knowledge about the current imports and namespace. It provides utilities for generating
// appropriately qualified names.
type Imports struct {
	currentGoPath string
	aliases       map[string]string
}

// NewImports creates a new Imports. Paths and aliases for the `importGoPaths` will be derived according to the
// `currentGoPath`
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

// CurPackage the current package for this Imports
func (i *Imports) CurPackage() string {
	return path.Base(i.currentGoPath)
}

// Import contains GoPath and Alias information, if any.
type Import struct {
	GoPath, Alias string
}

// List returns all of the imports in this value, ready to be rendered in a template. Aliases will be derived and
// provided as necessary.
func (i *Imports) List() (imports []Import) {
	for path, alias := range i.aliases {
		imports = append(imports, Import{path, alias})
	}
	sort.Slice(imports, func(i, j int) bool {
		return imports[i].GoPath < imports[j].GoPath
	})
	return
}

// QualName returns a qualified name for the provided TypeInfo; for example, if the value is from an imported package,
// QualName will return it with the appropriate dot prefix.
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

// TypeInfo encapsulates common information about a go value.
type TypeInfo struct {
	GoPath  string
	Name    string
	Pointer bool
	ValPath string
}

// BuiltIn returns whether or not this is a TypeInfo for a built-in type, where built-in means a primitive value type
// always available in the runtime
func (t TypeInfo) BuiltIn() bool {
	return t.GoPath == ""
}

// Unknown returns whether this TypeInfo is unset.
func (t TypeInfo) Unknown() bool {
	return t == TypeInfo{}
}
