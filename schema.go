package jsonschema2go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
)

var knownSchemaFields = make(map[string]bool)

func init() {
	for _, f := range getJSONFieldNames(Schema{}) {
		knownSchemaFields[f] = true
	}
}

type SimpleType uint8

const (
	Unknown = iota
	Array
	Boolean
	Integer
	Null
	Number
	Object
	String
)

var simpleTypeNames = map[string]SimpleType{
	"array":   Array,
	"boolean": Boolean,
	"integer": Integer,
	"null":    Null,
	"number":  Number,
	"object":  Object,
	"string":  String,
}

type TypeField []SimpleType

func (t *TypeField) UnmarshalJSON(b []byte) error {
	var val interface{}
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}
	switch v := val.(type) {
	case string:
		*t = append(*t, simpleTypeNames[v])
		return nil
	case []interface{}:
		*t = make(TypeField, 0, len(v))
		for _, v := range v {
			var typ SimpleType
			if s, ok := v.(string); ok {
				typ = simpleTypeNames[s]
			}
			*t = append(*t, typ)
		}
		return nil
	}
	return fmt.Errorf("unable to unmarshal %T into TypeField", val)
}

// convenience method to draw out the first token; if this errs, later calls will err anyway so discards
// the err
func peekToken(data []byte) json.Token {
	tok, _ := json.NewDecoder(bytes.NewReader(data)).Token()
	return tok
}

type BoolOrSchema struct {
	Bool   *bool
	Schema *Schema
}

func (a *BoolOrSchema) UnmarshalJSON(data []byte) error {
	if b, ok := peekToken(data).(bool); ok {
		a.Bool = &b
		return nil
	}
	return json.Unmarshal(data, a.Schema)
}

type ItemsFields struct {
	Items       *Schema
	TupleFields []*Schema
}

func (i *ItemsFields) UnmarshalJSON(data []byte) error {
	if peekToken(data) == json.Delim('{') {
		return json.Unmarshal(data, i.Items)
	}
	return json.Unmarshal(data, i.TupleFields)
}

type TagMap map[string]interface{}

func (t TagMap) GetString(k string) string {
	s, _ := t[k].(string)
	return s
}

type Schema struct {
	// this could be a ref
	Ref *string `json:"$ref,omitempty"`

	// meta
	ID     string `json:"id,omitempty"`
	Schema string `json:"$schema,omitempty"`

	// number qualifiers
	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMaximum *bool    `json:"exclusiveMaximum,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	ExclusiveMinimum *bool    `json:"exclusiveMinimum,omitempty"`

	// string qualifiers
	MaxLength *uint64 `json:"maxLength,omitempty"`
	MinLength uint64  `json:"minLength,omitempty"`
	Pattern   *string `json:"pattern,omitempty"`

	// array qualifiers
	AdditionalItems *BoolOrSchema `json:"additionalItems,omitempty"`
	Items           *ItemsFields  `json:"items,omitempty"`
	MaxItems        *uint64       `json:"maxItems,omitempty"`
	MinItems        uint64        `json:"minItems,omitempty"`
	UniqueItems     bool          `json:"uniqueItems,omitempty"`

	// object qualifiers
	MaxProperties        *uint64            `json:"maxProperties,omitempty"`
	MinProperties        uint64             `json:"minProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties *BoolOrSchema      `json:"additionalProperties,omitempty"`
	Definitions          map[string]*Schema `json:"definitions,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	PatternProperties    map[string]*Schema `json:"patternProperties,omitempty"`
	Dependencies         map[string]*Schema `json:"dependencies,omitempty"`

	// extra special
	Enum   []interface{} `json:"enum,omitempty"`
	Type   *TypeField    `json:"type,omitempty"`
	Format string        `json:"format,omitempty"`

	// polymorphic support
	AllOf []*Schema `json:"allOf,omitempty"`
	AnyOf []*Schema `json:"anyOf,omitempty"`
	OneOf []*Schema `json:"oneOf,omitempty"`
	Not   *Schema   `json:"not,omitempty"`

	// user extensible
	Annotations TagMap `json:"-"`
}

func (s *Schema) ChooseType() (t SimpleType) {
	if s.Type != nil && len(*s.Type) > 0 {
		t = (*s.Type)[0]
	}
	return
}

func (s *Schema) UnmarshalJSON(data []byte) error {
	type schema Schema

	var s2 schema
	if err := json.Unmarshal(data, &s2); err != nil {
		return fmt.Errorf("unmarshal schema: %w", err)
	}
	*s = Schema(s2)

	var possAnnos map[string]interface{}
	if err := json.Unmarshal(data, &possAnnos); err != nil {
		return fmt.Errorf("unmarshal annotations: %w", err)
	}

	for field, v := range possAnnos {
		if knownSchemaFields[field] {
			continue
		}
		if s.Annotations == nil {
			s.Annotations = make(map[string]interface{})
		}
		s.Annotations[field] = v
	}
	return nil
}

type HTTPDoer interface {
	Do(r *http.Request) (*http.Response, error)
}

var _ HTTPDoer = http.DefaultClient

func NewCachingLoader() *CachingLoader {
	return &CachingLoader{make(map[string]*Schema), http.DefaultClient}
}

type CachingLoader struct {
	cache  map[string]*Schema
	client HTTPDoer
}

func (c *CachingLoader) Load(ctx context.Context, s string) (*Schema, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("unable to parse %q: %w", s, err)
	}

	// cache check
	schema, ok := c.cache[u.String()]
	if ok {
		return schema, nil
	}
	defer func() {
		if schema != nil {
			c.cache[u.String()] = schema
		}
	}()

	// open IO
	var r io.ReadCloser
	switch u.Scheme {
	case "file":
		if r, err = os.Open(u.Path); err != nil {
			return nil, fmt.Errorf("unable to open %q: %w", u.Path, err)
		}
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create request for %q: %w", u, err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed requesting %q: %w", u, err)
		}
		r = resp.Body
	default:
		return nil, fmt.Errorf("unsupported scheme: %v", u.Scheme)
	}
	defer func() {
		_ = r.Close()
	}()

	if err := json.NewDecoder(r).Decode(schema); err != nil {
		return nil, fmt.Errorf("unable to decode %q: %w", u.Path, err)
	}

	return schema, nil
}

type Loader interface {
	Load(ctx context.Context, s string) (*Schema, error)
}

func Resolve(ctx context.Context, loader Loader, u string) (*Schema, error) {
	root, err := loader.Load(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("unable to load %v: %w", u, err)
	}

	type SchemaPath struct {
		URL    string
		Schema *Schema
	}

	var stack []SchemaPath

	push := func(s *Schema, components ...interface{}) {
		stack = append(stack, SchemaPath{u, s})
	}
	pop := func() *Schema {
		schemaURL := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		u = schemaURL.URL
		return schemaURL.Schema
	}
	resolve := func(u2 string) (string, error) {
		parsed, err := url.Parse(u)
		if err != nil {
			return "", err
		}

		parsed2, err := url.Parse(u2)
		if err != nil {
			return "", err
		}
		return parsed.ResolveReference(parsed2).String(), nil
	}
	push(root)
	for len(stack) > 0 {
		schema := pop()

		if schema.Ref != nil {
			if u, err = resolve(*schema.Ref); err != nil {
				return nil, err
			}
			s, err := loader.Load(ctx, u)
			if err != nil {
				return nil, fmt.Errorf("loading schema at %v failed: %v", u, err)
			}
			*schema = *s
		}
		if schema.AdditionalItems != nil && schema.AdditionalItems.Schema != nil {
			push(schema.AdditionalItems.Schema, "additionalItems")
		}
		if schema.Items != nil {
			if schema.Items.Items != nil {
				push(schema.Items.Items, "items")
			}
			for i, s := range schema.Items.TupleFields {
				push(s, "items", i)
			}
		}
		if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
			push(schema.AdditionalProperties.Schema, "additionalProperties")
		}
		for _, m := range []struct {
			Key     string
			Schemas map[string]*Schema
		}{
			{"definitions", schema.Definitions},
			{"properties", schema.Properties},
			{"patternProperties", schema.PatternProperties},
			{"definitions", schema.Definitions},
		} {
			for k, s := range m.Schemas {
				push(s, m.Key, k)
			}
		}
		for _, m := range []struct {
			Key     string
			Schemas []*Schema
		}{
			{"allOf", schema.AllOf},
			{"anyOf", schema.AnyOf},
			{"oneOf", schema.OneOf},
		} {
			for i, s := range m.Schemas {
				push(s, m.Key, i)
			}
		}
		if schema.Not != nil {
			push(schema.Not, "not")
		}
	}

	return root, nil
}

func getJSONFieldNames(val interface{}) (fields []string) {
	t := reflect.TypeOf(val)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		vals := strings.SplitN(field.Tag.Get("json"), ",", 2)
		if len(vals) == 0 || vals[0] == "" {
			fields = append(fields, field.Name)
			continue
		}
		if vals[0] != "-" {
			fields = append(fields, vals[0])
		}
	}
	return
}
