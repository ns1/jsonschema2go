[![Build Status](https://travis-ci.com/jwilner/jsonschema2go.svg?branch=master)](https://travis-ci.com/jwilner/jsonschema2go)
[![Coverage Status](https://coveralls.io/repos/github/jwilner/jsonschema2go/badge.svg?branch=master)](https://coveralls.io/github/jwilner/jsonschema2go?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jwilner/jsonschema2go)](https://goreportcard.com/report/github.com/jwilner/jsonschema2go)

# jsonschema2go

Generate Go types from JSON Schema files. Designed to be configurable to permit custom handling of different schema patterns.

Given a schema like in `example.json`:
```json
{
  "description": "Bar gives you some dumb info",
  "x-jsonschema2go": {
    "gopath": "github.com/jwilner/jsonschema2go/internal/example/foo#Bar"
  },
  "properties": {
    "baz": {
      "type": "string"
    },
    "boz": {
      "x-jsonschema2go": {
        "gopath": "github.com/jwilner/jsonschema2go/internal/example/foo#Boz"
      },
      "type": "object",
      "properties": {
        "count": "integer"
      }
    }
  }
}
```

`jsonschema2go` generates types like:
```go
package foo

// Bar gives you some dumb info
type Bar struct {
    Baz string  `json:"baz,omitempty"`
    Boz Boz     `json:"boz,omitempty"`
}

type Boz struct {
    Count int   `json:"count,omitempty"`
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
