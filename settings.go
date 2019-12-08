package jsonschema2go

import (
	"github.com/jwilner/jsonschema2go/internal/planning"
	"github.com/jwilner/jsonschema2go/internal/print"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	"github.com/jwilner/jsonschema2go/pkg/schema"
	"text/template"
)

type Option func(s *settings)

type settings struct {
	prefixes [][2]string
	typer    planning.Typer
	planner  generate.Planner
	printer  print.Printer
	loader   schema.Loader
	debug    bool
}

func PrefixMap(pairs ...string) Option {
	prefixes := prefixPairs(pairs)
	return func(s *settings) {
		s.prefixes = prefixes
	}
}

// Debug enables debug logging
func Debug(opt bool) Option {
	return func(s *settings) {
		s.debug = opt
	}
}

func CustomTypeFunc(typeFunc func(schema *schema.Schema) generate.TypeInfo) Option {
	return func(s *settings) {
		s.typer.TypeFunc = typeFunc
	}
}

func CustomPrimitivesMap(primitivesMap map[schema.SimpleType]string) Option {
	return func(s *settings) {
		s.typer.Primitives = primitivesMap
	}
}

func CustomPlanners(planners ...generate.Planner) Option {
	return func(s *settings) {
		s.planner = planning.CompositePlanner(planners)
	}
}

func TypeFromID(pairs ...string) Option {
	mapper := print.TypeFromId(prefixPairs(pairs))
	return func(s *settings) {
		s.typer.TypeFunc = func(schema *schema.Schema) generate.TypeInfo {
			if t := planning.DefaultTypeFunc(schema); !t.Unknown() {
				return t
			}
			if schema.CalcID != nil {
				if path, name := mapper(schema.CalcID.String()); name != "" {
					return generate.TypeInfo{GoPath: path, Name: name}
				}
			}
			return generate.TypeInfo{}
		}
	}
}

func CustomTemplate(tmpl *template.Template) Option {
	return func(s *settings) {
		s.printer = print.New(tmpl)
	}
}

func CustomInitialisms(names ...string) Option {
	return func(s *settings) {
		s.typer.Namer = planning.NewNamer(append(names, "id", "http"))
	}
}

func prefixPairs(pairs []string) [][2]string {
	if len(pairs)%2 != 0 {
		panic("must be even list of prefixes")
	}
	var prefixes [][2]string
	for i := 0; i < len(pairs); i += 2 {
		prefixes = append(prefixes, [2]string{pairs[i], pairs[i+1]})
	}
	return prefixes
}
