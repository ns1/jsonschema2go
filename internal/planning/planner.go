package planning

import (
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/composite"
	"github.com/jwilner/jsonschema2go/internal/enum"
	"github.com/jwilner/jsonschema2go/internal/slice"
	"github.com/jwilner/jsonschema2go/internal/tuple"
	"github.com/jwilner/jsonschema2go/pkg/gen"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	Composite = CompositePlanner{
		plannerFunc("allOfObject", composite.PlanAllOfObject),
		plannerFunc("object", composite.PlanObject),
		plannerFunc("tuple", tuple.PlanTuple),
		plannerFunc("slice", slice.Build),
		plannerFunc("enum", enum.Build),
		plannerFunc("discriminatedOneOf", composite.PlanDiscriminatedOneOfObject),
		plannerFunc("oneOfDiffTypes", composite.PlanOneOfDiffTypes),
	}
)

type CompositePlanner []gen.Planner

func (c CompositePlanner) Plan(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	for i, p := range c {
		name := strconv.Itoa(i)
		if p, ok := p.(interface{ Name() string }); ok {
			name = p.Name()
		}

		pl, err := p.Plan(ctx, helper, schema)
		if errors.Is(err, gen.ErrContinue) {
			if gen.IsDebug(ctx) {
				log.Printf("planner %v for %v: %v", name, schema, err)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if pl != nil {
			if gen.IsDebug(ctx) {
				log.Printf("planner %v: planned %v %v", name, pl.Type().GoPath, pl.Type().Name)
			}
			return pl, nil
		}
		return nil, fmt.Errorf("planner %v returned nil for plan", name)
	}
	// we require types for objects and arrays
	if t := schema.ChooseType(); t == gen.Object || t == gen.Array {
		return nil, fmt.Errorf("unable to plan %v", schema)
	}
	return nil, nil
}

func plannerFunc(
	name string,
	f func(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error),
) gen.Planner {
	return namedPlannerFunc{name: name, f: f}
}

type namedPlannerFunc struct {
	f    func(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error)
	name string
}

func (p namedPlannerFunc) Name() string {
	return p.name
}

func (p namedPlannerFunc) Plan(ctx context.Context, helper gen.Helper, schema *gen.Schema) (gen.Plan, error) {
	return p.f(ctx, helper, schema)
}

func NewHelper(
	ctx context.Context,
	loader gen.Loader,
	typer Typer,
	schemas <-chan *gen.Schema,
) *Helper {
	// allSchemas represents the merged stream of explicitly requested schemas and their children; it is
	// in essence the queue which powers a breadth-first search of the object graph
	allSchemas := make(chan *gen.Schema)
	// puts all schemas on merged and puts a signal on noMoreComing when no more coming
	noMoreComing := copyAndSignal(ctx, schemas, allSchemas)

	return &Helper{loader, typer, allSchemas, noMoreComing}
}

func copyAndSignal(ctx context.Context, schemas <-chan *gen.Schema, merged chan<- *gen.Schema) <-chan struct{} {
	schemasDone := make(chan struct{})
	go func() {
		for {
			select {
			case s, ok := <-schemas:
				if !ok {
					select {
					case schemasDone <- struct{}{}:
					case <-ctx.Done():
					}
					return
				}
				select {
				case merged <- s:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return schemasDone
}

type Helper struct {
	gen.Loader
	Typer
	Deps      chan *gen.Schema
	submitted <-chan struct{}
}

func (p *Helper) Schemas() <-chan *gen.Schema {
	return p.Deps
}

func (p *Helper) Submitted() <-chan struct{} {
	return p.submitted
}

func (p *Helper) Dep(ctx context.Context, schemas ...*gen.Schema) error {
	for _, s := range schemas {
		select {
		case p.Deps <- s:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func NewNamer(knownInitialisms []string) *Namer {
	m := make(map[string]bool)
	for _, n := range knownInitialisms {
		m[n] = true
	}
	return &Namer{m}
}

type Namer struct {
	knownInitialisms map[string]bool
}

func (n *Namer) JSONPropertyExported(name string) string {
	if strings.ToUpper(name) == name {
		return n.exportedIdentifier([][]rune{[]rune(strings.ToLower(name))})
	}

	var (
		current []rune
		parts   [][]rune
	)
	// split words
	for _, r := range []rune(name) {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			// exclusive word boundary
			if len(current) != 0 {
				parts = append(parts, current)
				current = nil
			}
			continue
		}
		if unicode.IsUpper(r) {
			// inclusive word boundary
			if len(current) != 0 {
				parts = append(parts, current)
			}
			current = []rune{unicode.ToLower(r)}
			continue
		}

		current = append(current, r)
	}

	if len(current) > 0 {
		parts = append(parts, current)
	}

	return n.exportedIdentifier(parts)
}

func (n *Namer) exportedIdentifier(parts [][]rune) string {
	var words []string
	for _, rs := range parts {
		if word := string(rs); n.knownInitialisms[word] {
			words = append(words, strings.ToUpper(word))
			continue
		}
		rs[0] = unicode.ToUpper(rs[0])
		words = append(words, string(rs))
	}
	return strings.Join(words, "")
}

var DefaultTyper = Typer{NewNamer([]string{"id", "http"}), MakeTypeFromID(nil), map[gen.SimpleType]string{
	gen.Boolean: "bool",
	gen.Integer: "int64",
	gen.Number:  "float64",
	gen.Null:    "interface{}",
	gen.String:  "string",
}}

func DefaultTypeFunc(s *gen.Schema) gen.TypeInfo {
	parts := strings.SplitN(s.Config.GoPath, "#", 2)
	if len(parts) == 2 {
		return gen.TypeInfo{GoPath: parts[0], Name: parts[1]}
	}
	return gen.TypeInfo{}
}

func MakeTypeFromID(pairs [][2]string) func(s *gen.Schema) gen.TypeInfo {
	// TypeFromID defines how to map to type information from IDs
	mapper := TypeFromId(pairs)
	return func(schema *gen.Schema) gen.TypeInfo {
		if t := DefaultTypeFunc(schema); !t.Unknown() {
			return t
		}
		if path, name := mapper(schema.ID.String()); name != "" {
			return gen.TypeInfo{GoPath: path, Name: name}
		}
		return gen.TypeInfo{}
	}
}

type Typer struct {
	*Namer
	TypeFunc   func(s *gen.Schema) gen.TypeInfo
	Primitives map[gen.SimpleType]string
}

func (d Typer) TypeInfo(s *gen.Schema) gen.TypeInfo {
	t := s.ChooseType()
	if t != gen.Array && t != gen.Object && s.Config.GoPath == "" {
		return gen.TypeInfo{Name: d.Primitive(t)}
	}
	return d.TypeInfoHinted(s, t)
}

func (d Typer) TypeInfoHinted(s *gen.Schema, t gen.SimpleType) gen.TypeInfo {
	if f := d.TypeFunc(s); f.Name != "" {
		f.Name = d.Namer.JSONPropertyExported(f.Name)
		return f
	}
	return gen.TypeInfo{Name: d.Primitive(t)}
}

func (d Typer) Primitive(s gen.SimpleType) string {
	return d.Primitives[s]
}

func TypeFromId(pairs [][2]string) func(string) (string, string) {
	mapper := PrefixMapper(pairs)
	return func(s string) (string, string) {
		s = mapper(s)
		u, err := url.Parse(s)
		if err != nil {
			return "", ""
		}
		pathParts := strings.Split(u.Host+u.Path, "/")
		if len(pathParts) < 2 {
			return "", ""
		}
		// drop the extension
		nameParts := strings.SplitN(pathParts[len(pathParts)-1], ".", 2)
		if len(nameParts) == 0 {
			return "", ""
		}
		path, name := strings.Join(pathParts[:len(pathParts)-1], "/"), nameParts[0]
		// add any fragment info
		if u.Fragment != "" {
			frags := strings.Split(u.Fragment, "/")
			for _, frag := range frags {
				if frag == "" || frag == "properties" {
					continue
				}
				runes := []rune(frag)
				runes[0] = unicode.ToUpper(runes[0])
				name += string(runes)
			}
		}
		return path, name
	}
}

func PrefixMapper(prefixes [][2]string) func(string) string {
	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i][0] < prefixes[j][0]
	})
	return func(path string) string {
		i := sort.Search(len(prefixes), func(i int) bool {
			return prefixes[i][0] > path
		})
		for i = i - 1; i >= 0; i-- {
			if strings.HasPrefix(path, prefixes[i][0]) {
				return prefixes[i][1] + path[len(prefixes[i][0]):]
			}
		}
		return path
	}
}
