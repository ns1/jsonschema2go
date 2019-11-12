[![Build Status](https://travis-ci.com/jwilner/json2go.svg?branch=master)](https://travis-ci.com/jwilner/json2go)
[![Coverage Status](https://coveralls.io/repos/github/jwilner/json2go/badge.svg?branch=master)](https://coveralls.io/github/jwilner/json2go?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/jwilner/json2go)](https://goreportcard.com/report/github.com/jwilner/json2go)

# json2go

Generate Go types from JSON Schema files. Designed to be configurable to permit custom handling of different schema patterns.

Given a schema like in `example.json`:
```json
{
  "description": "Bar gives you some dumb info",
  "x-json2go": {
    "gopath": "github.com/jwilner/json2go/internal/example/foo#Bar"
  },
  "properties": {
    "baz": {
      "type": "string"
    },
    "boz": {
      "x-json2go": {
        "gopath": "github.com/jwilner/json2go/internal/example/foo#Boz"
      },
      "type": "object",
      "properties": {
        "count": "integer"
      }
    }
  }
}
```

`json2go` generates types like:
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

Default configuration for `json2go` handles primitives, arrays, objects, `allOf` polymorphism, and `oneOf` polymorphism (with discriminator annotation).

## Usage
```go
package main

import (
    "context"
    "log"

    "github.com/jwilner/json2go"
)

func main() {
    if err := json2go.Generate(context.Background(), []string{"example.json"}); err != nil {
        log.Fatal(err)
    }
}
```
