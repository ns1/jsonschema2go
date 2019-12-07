[![Build Status](https://travis-ci.com/jwilner/jsonschema2go.svg?branch=master)](https://travis-ci.com/jwilner/jsonschema2go)
[![Coverage Status](https://coveralls.io/repos/github/jwilner/jsonschema2go/badge.svg?branch=master)](https://coveralls.io/github/jwilner/jsonschema2go?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jwilner/jsonschema2go)](https://goreportcard.com/report/github.com/jwilner/jsonschema2go)

# jsonschema2go

Generate Go types from JSON Schema files. Designed to be configurable to permit custom handling of different schema patterns.

Given a schema like in `example.json`:
```json
{
  "description": "Bar contains some info",
  "x-jsonschema2go": {
    "gopath": "github.com/jwilner/jsonschema2go/internal/testdata/render/example/foo#Bar"
  },
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
type Bar struct {
	Baz   boxed.String `json:"baz"`
	Count boxed.Int64  `json:"count"`
}

var (
	barBazPattern = regexp.MustCompile(`^[0-9a-fA-F]{10}$`)
)

func (m *Bar) Validate() error {
	if !m.Baz.Set {
		return &validationError{
			errType:  "required",
			message:  "field required",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
		}
	}
	if !barBazPattern.MatchString(m.Baz.String) {
		return &validationError{
			errType:  "pattern",
			path:     []interface{}{"Baz"},
			jsonPath: []interface{}{"baz"},
			message:  fmt.Sprintf("must match '^[0-9a-fA-F]{10}$' but got %q", m.Baz.String),
		}
	}
	if m.Count.Set && m.Count.Int64 < 3 {
		return &validationError{
			errType:  "minimum",
			path:     []interface{}{"Count"},
			jsonPath: []interface{}{"count"},
			message:  fmt.Sprintf("must be greater than or equal to 3 but was %v", m.Count.Int64),
		}
	}
	return nil
}
```

## Types

Default configuration for `jsonschema2go` handles primitives, arrays, objects, `allOf` polymorphism, and `oneOf` polymorphism (with discriminator annotation).

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
