package planning

import (
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/composite"
	"github.com/jwilner/jsonschema2go/internal/enum"
	"github.com/jwilner/jsonschema2go/internal/slice"
	"github.com/jwilner/jsonschema2go/internal/tuple"
	"github.com/jwilner/jsonschema2go/pkg/ctxflags"
	"github.com/jwilner/jsonschema2go/pkg/generate"
	sch "github.com/jwilner/jsonschema2go/pkg/schema"
	"log"
	"strconv"
	"strings"
	"unicode"
)

var (
	Composite = CompositePlanner{
		plannerFunc("allOfObject", composite.PlanAllOfObject),
		plannerFunc("object", composite.PlanObject),
		plannerFunc("tuple", tuple.PlanTuple),
		plannerFunc("slice", slice.PlanSlice),
		plannerFunc("enum", enum.Plan),
		plannerFunc("discriminatedOneOf", composite.PlanDiscriminatedOneOfObject),
		plannerFunc("oneOfDiffTypes", composite.PlanOneOfDiffTypes),
	}
)

type CompositePlanner []generate.Planner

//go:generate go run ../cmd/embedtmpl/embedtmpl.go planning values.tmpl tmpl.gen.go
func (c CompositePlanner) Plan(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	for i, p := range c {
		name := strconv.Itoa(i)
		if p, ok := p.(interface{ Name() string }); ok {
			name = p.Name()
		}

		pl, err := p.Plan(ctx, helper, schema)
		if errors.Is(err, generate.ErrContinue) {
			if ctxflags.IsDebug(ctx) {
				log.Printf("planner %v: skipping planner: %v", name, err)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if pl != nil {
			if ctxflags.IsDebug(ctx) {
				log.Printf("planner %v: planned %v %v", name, pl.Type().GoPath, pl.Type().Name)
			}
			return pl, nil
		}
		return nil, fmt.Errorf("planner %v returned nil for plan", name)
	}
	// we require types for objects and arrays
	if t := schema.ChooseType(); t == sch.Object || t == sch.Array {
		id := schema.Loc
		if schema.CalcID != nil {
			id = schema.CalcID
		}
		return nil, fmt.Errorf("unable to plan %v", id)
	}
	return nil, nil
}

func plannerFunc(
	name string,
	f func(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error),
) generate.Planner {
	return namedPlannerFunc{name: name, f: f}
}

type namedPlannerFunc struct {
	f    func(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error)
	name string
}

func (p namedPlannerFunc) Name() string {
	return p.name
}

func (p namedPlannerFunc) Plan(ctx context.Context, helper generate.Helper, schema *sch.Schema) (generate.Plan, error) {
	return p.f(ctx, helper, schema)
}

func NewHelper(
	ctx context.Context,
	loader sch.Loader,
	typer Typer,
	schemas <-chan *sch.Schema,
) *Helper {
	// allSchemas represents the merged stream of explicitly requested schemas and their children; it is
	// in essence the queue which powers a breadth-first search of the object graph
	allSchemas := make(chan *sch.Schema)
	// puts all schemas on merged and puts a signal on noMoreComing when no more coming
	noMoreComing := copyAndSignal(ctx, schemas, allSchemas)

	return &Helper{loader, typer, allSchemas, noMoreComing}
}

func copyAndSignal(ctx context.Context, schemas <-chan *sch.Schema, merged chan<- *sch.Schema) <-chan struct{} {
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
	sch.Loader
	Typer
	Deps      chan *sch.Schema
	submitted <-chan struct{}
}

func (p *Helper) Schemas() <-chan *sch.Schema {
	return p.Deps
}

func (p *Helper) Submitted() <-chan struct{} {
	return p.submitted
}

func (p *Helper) Dep(ctx context.Context, schemas ...*sch.Schema) error {
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

var DefaultTyper = Typer{NewNamer([]string{"id", "http"}), DefaultTypeFunc, map[sch.SimpleType]string{
	sch.Boolean: "bool",
	sch.Integer: "int64",
	sch.Number:  "float64",
	sch.Null:    "interface{}",
	sch.String:  "string",
}}

func DefaultTypeFunc(s *sch.Schema) generate.TypeInfo {
	parts := strings.SplitN(s.Config.GoPath, "#", 2)
	if len(parts) == 2 {
		return generate.TypeInfo{GoPath: parts[0], Name: parts[1]}
	}
	return generate.TypeInfo{}
}

type Typer struct {
	*Namer
	TypeFunc   func(s *sch.Schema) generate.TypeInfo
	Primitives map[sch.SimpleType]string
}

func (d Typer) TypeInfo(s *sch.Schema) generate.TypeInfo {
	t := s.ChooseType()
	if t != sch.Array && t != sch.Object && s.Config.GoPath == "" {
		return generate.TypeInfo{Name: d.Primitive(t)}
	}
	return d.TypeInfoHinted(s, t)
}

func (d Typer) TypeInfoHinted(s *sch.Schema, t sch.SimpleType) generate.TypeInfo {
	if f := d.TypeFunc(s); f.Name != "" {
		f.Name = d.Namer.JSONPropertyExported(f.Name)
		return f
	}
	return generate.TypeInfo{Name: d.Primitive(t)}
}

func (d Typer) Primitive(s sch.SimpleType) string {
	return d.Primitives[s]
}
