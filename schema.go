package jsonschema2go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
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
	Schema *RefOrSchema
}

func (a *BoolOrSchema) UnmarshalJSON(data []byte) error {
	if b, ok := peekToken(data).(bool); ok {
		a.Bool = &b
		return nil
	}
	a.Schema = new(RefOrSchema)
	return json.Unmarshal(data, a.Schema)
}

type ItemsFields struct {
	Items       *RefOrSchema
	TupleFields []*RefOrSchema
}

func (i *ItemsFields) UnmarshalJSON(data []byte) error {
	if peekToken(data) == json.Delim('{') {
		i.Items = new(RefOrSchema)
		return json.Unmarshal(data, i.Items)
	}
	return json.Unmarshal(data, &i.TupleFields)
}

type TagMap map[string]json.RawMessage

func (t TagMap) GetString(k string) (s string) {
	_, _ = t.Unmarshal(k, &s)
	return
}

func (t TagMap) Unmarshal(k string, val interface{}) (bool, error) {
	msg, ok := t[k]
	if !ok {
		return false, nil
	}
	err := json.Unmarshal(msg, &val)
	return true, err
}

type RefOrSchema struct {
	ref    *string
	schema *Schema
	curLoc *url.URL
}

func (r *RefOrSchema) UnmarshalJSON(b []byte) error {
	var ref struct {
		Ref string `json:"$ref"`
	}
	if err := json.Unmarshal(b, &ref); err != nil {
		return fmt.Errorf("unmarshal $ref: %w", err)
	}
	if ref.Ref != "" {
		r.ref = &ref.Ref
		return nil
	}
	r.schema = new(Schema)
	return json.Unmarshal(b, r.schema)
}

func (r *RefOrSchema) Resolve(ctx context.Context, referer *Schema, loader Loader) (*Schema, error) {
	if r.ref == nil {
		return r.schema, nil
	}

	parsed2, err := url.Parse(*r.ref)
	if err != nil {
		return nil, fmt.Errorf("parse $ref: %w", err)
	}

	return loader.Load(ctx, referer.curLoc.ResolveReference(parsed2))
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
	MaxProperties        *uint64                 `json:"maxProperties,omitempty"`
	MinProperties        uint64                  `json:"minProperties,omitempty"`
	Required             []string                `json:"required,omitempty"`
	AdditionalProperties *BoolOrSchema           `json:"additionalProperties,omitempty"`
	Definitions          map[string]*RefOrSchema `json:"definitions,omitempty"`
	Properties           map[string]*RefOrSchema `json:"properties,omitempty"`
	PatternProperties    map[string]*RefOrSchema `json:"patternProperties,omitempty"`
	Dependencies         map[string]*RefOrSchema `json:"dependencies,omitempty"`

	// extra special
	Enum   []interface{} `json:"enum,omitempty"`
	Type   *TypeField    `json:"type,omitempty"`
	Format string        `json:"format,omitempty"`

	// polymorphic support
	AllOf []*RefOrSchema `json:"allOf,omitempty"`
	AnyOf []*RefOrSchema `json:"anyOf,omitempty"`
	OneOf []*RefOrSchema `json:"oneOf,omitempty"`
	Not   *RefOrSchema   `json:"not,omitempty"`

	// jsonschema2go config
	Config config `json:"x-jsonschema2go"`

	// user extensible
	Annotations TagMap `json:"-"`

	// curLoc -- internal bookkeeping, the resource from which this schema was loaded
	curLoc *url.URL
}

type config struct {
	GoPath        string        `json:"gopath"`
	Exclude       bool          `json:"exclude"`
	Discriminator discriminator `json:"discriminator"`
}

type discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping"`
}

func (d *discriminator) isSet() bool {
	return d.PropertyName != ""
}

func (s *Schema) setCurLoc(u *url.URL) {
	schemas := []*Schema{s}
	push := func(r *RefOrSchema) {
		if r != nil && r.schema != nil {
			schemas = append(schemas, r.schema)
		}
	}

	for len(schemas) > 0 {
		s := schemas[0]
		schemas = schemas[1:]

		s.curLoc = u
		if s.AdditionalItems != nil {
			push(s.AdditionalItems.Schema)
		}
		if s.Items != nil {
			push(s.Items.Items)
			for _, f := range s.Items.TupleFields {
				push(f)
			}
		}
		if s.AdditionalProperties != nil {
			push(s.AdditionalProperties.Schema)
		}
		for _, m := range []map[string]*RefOrSchema{
			s.Definitions,
			s.Properties,
			s.PatternProperties,
			s.Dependencies,
		} {
			for _, v := range m {
				push(v)
			}
		}
		for _, a := range [][]*RefOrSchema{
			s.AllOf,
			s.AnyOf,
			s.OneOf,
		} {
			for _, v := range a {
				push(v)
			}
		}
		push(s.Not)
	}
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

	var possAnnos map[string]json.RawMessage
	if err := json.Unmarshal(data, &possAnnos); err != nil {
		return fmt.Errorf("unmarshal annotations: %w", err)
	}

	for field, v := range possAnnos {
		if knownSchemaFields[field] {
			continue
		}
		if s.Annotations == nil {
			s.Annotations = make(map[string]json.RawMessage)
		}
		s.Annotations[field] = v
	}
	return nil
}

func (s *Schema) Meta() SchemaMeta {
	return SchemaMeta{
		ID:          s.ID,
		BestType:    s.ChooseType(),
		Annotations: s.Annotations,
		Flags:       s.Config,
	}
}

type SchemaMeta struct {
	ID          string
	BestType    SimpleType
	Annotations TagMap
	Flags       config
}

type Loader interface {
	Load(ctx context.Context, u *url.URL) (*Schema, error)
}

func getJSONFieldNames(val interface{}) (fields []string) {
	t := reflect.TypeOf(val)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if r, _ := utf8.DecodeRuneInString(field.Name); r == utf8.RuneError || unicode.IsLower(r) {
			continue
		}
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
