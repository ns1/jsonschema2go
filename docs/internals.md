# `jsonschema2go` internals

It generates Go types from `jsonschema` definitions. It does not support the full breadth of `jsonschema`, nor does it aim to; `jsonschema` isn't designed as a DDL, so there are many valid expressions within it that you would never be useful as message definitions.

## Flow

![flow](flow.svg)

### Input

User provides URLs for one or more JSON schema that they want to generate code from along with a series of options for interpreting them.

```go
// Generate generates Go source code from the provided JSON schemas. Options can be provided to customize the
// output behavior
func Generate(ctx context.Context, uris []string, options ...Option) error {
```

### Crawling

Each json schema is recursive in its definition, so usually what appears to be a single schema is actually a graph of related schemas.

For example, the below is actually three schemas:

1) The wrapping array
2) The object schema for the item
3) The int schema

```json
{
  "type": "array",
  "items": {
    "type": "object",
    "properties": {
      "field": {
        "type": "int"
      }
    }
  }
}
```

The crawler performs a BFS where the queue is initialized with the toplevel schemas from the user input. Note that because it's BFS, it's not easy for child types to influence the rendering of parent types -- you've always done a parent before its children.

It calls the **planning** logic for each schema in the queue; the planning logic is responsible for generating a plan but also for reporting any new subschemas which a plan will rely on. The reported subschemas are added back into the BFS queue.

`jsonschema2go` generates a plan -- a Go type and validation methods -- for every schema corresponding to a non-scalar type; each of these schemas is guaranteed to be visited only once.

Scalar subschemas are handled as fields on structs, items in slices, or items in tuples and are validated by their containing object.

As a result, there is no support for top level scalar subschemas.

### Planning

A `plan` is the intervening object between the schema and a rendered type; a plan corresponds to a template.

```go
// Plan is the contract that must be filled for a type to be rendered.
type Plan interface {
	// Type returns the TypeInfo for the current type
	Type() TypeInfo
	// Deps returns any dependent types for the current type (i.e. any types requiring import)
	Deps() []TypeInfo
	// Execute is provided a resolved imports and should provide the type rendered as a string.
	Execute(imports *Imports) (string, error)
}
```

The rest of the application only knows about a `CompositePlanner`, which in turn uses one of seven planner functions by visiting them and trying them in order.

```
Composite = CompositePlanner{
		plannerFunc("map", mapobj.PlanMap),
		plannerFunc("allOfObject", composite.PlanAllOfObject),
		plannerFunc("object", composite.PlanObject),
		plannerFunc("tuple", tuple.PlanTuple),
		plannerFunc("slice", slice.Build),
		plannerFunc("discriminatedOneOf", composite.PlanDiscriminatedOneOfObject),
		plannerFunc("oneOfDiffTypes", composite.PlanOneOfDiffTypes),
}
```

Each planner can either return a) a plan, b) a skip signal, or c) an error. If every planner returns a skip signal for the provided schema, the generation fails.

Planning derives type information and validation information but is also responsible for generating

Additionally, as mentioned before, every planner is responsible for requesting subtypes.

### Printing

Before printing, `Plan`s are grouped according to their destination package. An `Imports` type representing the namespace and providing utilities for deriving qualified names is created, which is then passed to each `Plan`'s `Execute` method.

Each `Plan` uses the `Imports` object to render its type as a string, which are then all fastened together

## Tests

There are two main sorts of tests, both of which are driven by test data files.

### generate tests

`generate` tests go from one or more schemas to source code; they validate that the generated code looks as we expected.

### validation tests

`validate` tests go from one or more schemas to a compiled binary (in a harness) and then runs that compiled binary against the reference test suite from the `jsonschema` specification. Because we don't support the entire specification, some parts are skipped.
