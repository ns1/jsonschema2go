[![Build Status](https://travis-ci.com/jwilner/jsonschema2go.svg?branch=master)](https://travis-ci.com/jwilner/jsonschema2go)
[![Coverage Status](https://coveralls.io/repos/github/jwilner/jsonschema2go/badge.svg?branch=master)](https://coveralls.io/github/jwilner/jsonschema2go?branch=master)
[![GoDoc](https://godoc.org/github.com/jwilner/jsonschema2go?status.svg)](https://godoc.org/github.com/jwilner/jsonschema2go)
[![Go Report Card](https://goreportcard.com/badge/github.com/jwilner/jsonschema2go)](https://goreportcard.com/report/github.com/jwilner/jsonschema2go)

# jsonschema2go

Generate Go types from JSON Schema files. Designed to be configurable to permit custom handling of different schema patterns.

Given a schema like in `example.json`:
```json
{
  "id": "https://example.com/testdata/generate/example/foo/bar.json",
  "description": "Bar contains some info",
  "properties": {
    "baz": {
      "type": "string",
      "pattern": "^[0-9a-fA-F]{10}$"
    },
    "count": {
      "type": "integer",
      "minimum": 3
    }
  },
  "required": ["baz"]
}
```

`jsonschema2go` generates types like:
```go
// Bar contains some info
// generated from https://example.com/testdata/generate/example/foo/bar.json
type Bar struct {
	Baz   *string `json:"baz,omitempty"`
	Count *int64  `json:"count,omitempty"`
}

var (
	barBazPattern = regexp.MustCompile(`^[0-9a-fA-F]{10}$`)
)

func (m *Bar) Validate() error {
	if m.Baz == nil {
		return &validationError{
			errType:  "required",
			message:  "field required",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
		}
	}
	if !barBazPattern.MatchString(*m.Baz) {
		return &validationError{
			errType:  "pattern",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
			message:  fmt.Sprintf(`must match '^[0-9a-fA-F]{10}$' but got %q`, *m.Baz),
		}
	}
	if m.Count != nil && *m.Count < 3 {
		return &validationError{
			errType:  "minimum",
			path:     []interface{}{"Count"},
			jsonPath: []interface{}{"count"},
			message:  fmt.Sprintf("must be greater than or equal to 3 but was %v", *m.Count),
		}
	}
	return nil
}
```

## Types

Default configuration for JSONSchema2Go handles a wide subset of the JSONSchema specification. For documentation of the coverage, consult the various test cases in all of the `testdata` directories.

## Usage
```go
package main

import (
    "context"
    "log"

    "github.com/jwilner/jsonschema2go"
)

func main() {
    if err := jsonschema2go.Generate(context.Background(), []string{"example.json"}); err != nil {
        log.Fatal(err)
    }
}
```

## Naming Rules

### Top level schemas

IDs are required for all top level schemas (this is already a best practice for JSONSchema). By default, that ID is used to generate type information. For example, if you provide a schema with the ID, `https://example.com/testdata/generate/example/foo/bar.json`, the default behavior is to generate a type `Bar` in the current working directory at `example.com/testdata/generate/example/foo`.

You can also provide an explicit Go path (and name) by setting `x-jsonschema2go.gopath`. For example, setting `gopath` to `example.com/examples/foo#Baz` will set the target gopath to `example.com/examples/foo` and the type name to `Baz`.

### Nested schemas

For nested schemas mapping to types which require names, if not explicitly set via ID or `x-jsonschema2go.gopath`, the name will be derived from the name of the containing top level spec and the path to the element. For example, if the type `Bar` has a field `Baz`, the field's type might be `BarBaz`.
