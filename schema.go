package jsonschema2go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	return json.Unmarshal(data, i.TupleFields)
}

type TagMap map[string]interface{}

func (t TagMap) GetString(k string) string {
	s, _ := t[k].(string)
	return s
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
		return err
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
		return nil, err
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

	// user extensible
	Annotations TagMap `json:"-"`

	// curLoc -- internal bookkeeping, the resource from which this schema was loaded
	curLoc *url.URL `json:"-"`
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

func (s *Schema) Meta() SchemaMeta {
	return SchemaMeta{
		ID:          s.ID,
		BestType:    s.ChooseType(),
		Annotations: s.Annotations,
	}
}

type SchemaMeta struct {
	ID          string
	BestType    SimpleType
	Annotations TagMap
}

type Loader interface {
	Load(ctx context.Context, u *url.URL) (*Schema, error)
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
