package crawl

import (
	"context"
	"errors"
	"fmt"
	"github.com/jwilner/jsonschema2go/internal/planning"
	gen "github.com/jwilner/jsonschema2go/pkg/gen"
	"net/url"
)

func Crawl(
	ctx context.Context,
	planner gen.Planner,
	loader gen.Loader,
	typer planning.Typer,
	uris []string,
) (map[string][]gen.Plan, error) {
	var schemas []*gen.Schema
	for _, uri := range uris {
		u, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("unable to parse %q: %w", uri, err)
		}
		sch, err := loader.Load(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("unable to load %q: %w", uri, err)
		}
		schemas = append(schemas, sch)
	}

	plans, err := crawl(ctx, loader, typer, planner, schemas)
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]gen.Plan)
	for _, p := range plans {
		grouped[p.Type().GoPath] = append(grouped[p.Type().GoPath], p)
	}
	return grouped, nil
}

func crawl(
	ctx context.Context,
	loader gen.Loader,
	typer planning.Typer,
	planner gen.Planner,
	schemas []*gen.Schema,
) ([]gen.Plan, error) {
	{
		q := make([]*gen.Schema, len(schemas))
		copy(q, schemas)
		schemas = q
	}

	var plans []gen.Plan
	seen := make(map[string]bool)
	for len(schemas) > 0 {
		s := schemas[0]
		schemas = schemas[1:]

		k := s.ID.String()
		if seen[k] {
			continue
		}
		seen[k] = true

		if s.Config.Exclude {
			continue
		}

		helper := &SimpleHelper{loader, typer, nil}
		p, err := planner.Plan(ctx, helper, s)
		if err != nil {
			return nil, fmt.Errorf("unable to plan %v: %w", s, err)
		}
		if p == nil {
			return nil, fmt.Errorf("received a nil plan for %v", s)
		}

		schemas = append(schemas, helper.deps...)
		plans = append(plans, p)
	}

	return plans, nil
}

type SimpleHelper struct {
	gen.Loader
	planning.Typer
	deps []*gen.Schema
}

func (h *SimpleHelper) Dep(ctx context.Context, schemas ...*gen.Schema) error {
	h.deps = append(h.deps, schemas...)
	return nil
}


func (h *SimpleHelper) DetectGoBaseType(ctx context.Context, schema *gen.Schema) (gen.GoBaseType, error) {
	if len(schema.AllOf) > 0 || len(schema.OneOf) > 0 || len(schema.AnyOf) > 0 {
		return gen.GoStruct, nil
	}

	jTyp, err := h.DetectSimpleType(ctx, schema)
	if err != nil {
		return gen.GoUnknown, err
	}
	switch jTyp {
	case gen.JSONBoolean:
		return gen.GoBool, nil
	case gen.JSONInteger:
		return gen.GoInt64, nil
	case gen.JSONNumber:
		return gen.GoFloat64, nil
	case gen.JSONString:
		return gen.GoString, nil
	case gen.JSONNull:
		return gen.GoEmpty, nil
	case gen.JSONArray:
		if schema.Items != nil && schema.Items.TupleFields != nil {
			return gen.GoArray, nil
		}
		return gen.GoSlice, nil
	case gen.JSONObject:
		if len(schema.Properties) > 0 {
			return gen.GoStruct, nil
		}
		return gen.GoMap, nil
	}
	return gen.GoUnknown, nil
}

func (h *SimpleHelper) DetectSimpleType(ctx context.Context, schema *gen.Schema) (gen.JSONType, error) {
	level := []*gen.Schema{schema}
	for len(level) > 0 {
		found := gen.JSONUnknown
		for _, s := range level {
			myT := s.ChooseType()
			if myT == gen.JSONUnknown {
				continue
			}
			if found != gen.JSONUnknown && found != myT {
				return gen.JSONUnknown, errors.New("conflicting type")
			}
			found = myT
		}
		if found != gen.JSONUnknown {
			return found, nil
		}

		var candidates []*gen.Schema
		for _, s := range level {
			for _, sub := range s.AllOf {
				c, err := sub.Resolve(ctx, s, h)
				if err != nil {
					return gen.JSONUnknown, err
				}
				candidates = append(candidates, c)
			}
		}

		level = candidates
	}
	return gen.JSONUnknown, fmt.Errorf("%v: %w", schema, errTypeUnknown)
}

var errTypeUnknown = errors.New("unknown type")

func (h *SimpleHelper) ErrSimpleTypeUnknown(err error) bool {
	return errors.Is(err, errTypeUnknown)
}