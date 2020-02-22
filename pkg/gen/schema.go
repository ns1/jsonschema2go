package gen

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

// SimpleType is the enumeration of JSONSchema's supported types.
type SimpleType uint8

// Each of these is a core type of JSONSchema, except for Unknown, which is a userful zero value.
const (
	Unknown SimpleType = iota
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

// TypeField wraps the type field in JSONSchema, supporting either an array of types or a single type as the metaschema
// allows
type TypeField []SimpleType

// UnmarshalJSON unmarshals JSON into the TypeField
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

// BoolOrSchema may have either a boolean or a RefOrSchema.
type BoolOrSchema struct {
	Bool   *bool
	Schema *RefOrSchema
}

func (a *BoolOrSchema) Present() bool {
	return a != nil && (a.Schema != nil || (a.Bool != nil && *a.Bool))
}

// UnmarshalJSON performs some custom deserialization of JSON into BoolOrSchema
func (a *BoolOrSchema) UnmarshalJSON(data []byte) error {
	if b, ok := peekToken(data).(bool); ok {
		a.Bool = &b
		return nil
	}
	a.Schema = new(RefOrSchema)
	return json.Unmarshal(data, a.Schema)
}

// ItemsField contains information indicating whether the modified array is a dynamically sized list of multiple
// types or a "tuple" -- a specifically sized array with potentially different types for each position.
type ItemsField struct {
	Items       *RefOrSchema
	TupleFields []*RefOrSchema
}

// UnmarshalJSON conditionally deserializes into ItemsField according to the shape of the provided JSON
func (i *ItemsField) UnmarshalJSON(data []byte) error {
	if peekToken(data) == json.Delim('{') {
		i.Items = new(RefOrSchema)
		return json.Unmarshal(data, i.Items)
	}
	return json.Unmarshal(data, &i.TupleFields)
}

// TagMap contains all of the different user extended tags as json.RawMessage for later deserialization
type TagMap map[string]json.RawMessage

// GetString attempts to deserialize the value for the provided key into a string. If the key is absent or there is an
// error deserializing the value, the returned string will be empty.
func (t TagMap) GetString(k string) (s string) {
	_, _ = t.Unmarshal(k, &s)
	return
}

// Read unmarshals the json at the provided key into the provided interface (which should be a pointer amenable to
// json.Read. If the key is not present, the pointer will be untouched, and false and nil will be returned. If the
// deserialization fails, an error will be returned.
func (t TagMap) Unmarshal(k string, val interface{}) (bool, error) {
	msg, ok := t[k]
	if !ok {
		return false, nil
	}
	err := json.Unmarshal(msg, val)
	return true, err
}

// NewRefOrSchema is a convenience constructor for RefOrSchema
func NewRefOrSchema(s *Schema, ref *string) *RefOrSchema {
	return &RefOrSchema{ref: ref, schema: s}
}

// RefOrSchema is either a schema or a reference to a schema.
type RefOrSchema struct {
	ref    *string
	schema *Schema
}

// UnmarshalJSON conditionally deserializes the JSON, either into a reference or a schema.
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

// Resolve either returns the schema if set or else resolves the reference using the referer schema and loader.
func (r *RefOrSchema) Resolve(ctx context.Context, referer *Schema, loader Loader) (*Schema, error) {
	if r.ref == nil {
		return r.schema, nil
	}

	parsed2, err := url.Parse(*r.ref)
	if err != nil {
		return nil, fmt.Errorf("parse $ref: %w", err)
	}

	return loader.Load(ctx, referer.Src.ResolveReference(parsed2))
}

// Schema is the core representation of the JSONSchema meta schema.
type Schema struct {
	// this could be a ref
	Ref *string `json:"$ref,omitempty"`

	// meta
	ID     *url.URL `json:"-"` // set either from "$id", "id", or calculated based on parent (see IDCalc); never nil
	IDCalc bool     `json:"-"` // whether this ID was calculated
	Src    *url.URL `json:"-"` // the resource from which this schema was loaded; never nil
	Schema string   `json:"$schema,omitempty"`

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
	Items           *ItemsField   `json:"items,omitempty"`
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

	// jsonschema2go Config
	Config Config `json:"x-jsonschema2go"`

	// user extensible
	Annotations TagMap `json:"-"`
}

// Config is a series of jsonschema2go user extensions
type Config struct {
	GoPath        string        `json:"gopath"`
	Exclude       bool          `json:"exclude"`
	Discriminator Discriminator `json:"Discriminator"`
	NoValidate    bool          `json:"noValidate"`
	PromoteFields bool          `json:"promoteFields"`
}

// Discriminator is jsonschema2go specific info for discriminating between multiple oneOf objects
type Discriminator struct {
	PropertyName string            `json:"propertyName"`
	Mapping      map[string]string `json:"mapping"`
}

// IsSet returns whether there is a discriminator present.
func (d *Discriminator) IsSet() bool {
	return d.PropertyName != ""
}

func (s *Schema) setSrc(u *url.URL) {
	s.Src = u
	for _, c := range s.children() {
		if c.schema != nil {
			c.schema.setSrc(u)
		}
	}
}

func (s *Schema) calculateID() {
	for _, c := range s.children() {
		if c.schema == nil {
			continue
		}
		if c.schema.ID == nil {
			childID, _ := s.ID.Parse(s.ID.String()) // silly deep copy
			if len(c.path) > 0 {
				fragment := make([]string, 0, len(c.path))
				for _, v := range c.path {
					fragment = append(fragment, fmt.Sprint(v))
				}
				childID.Fragment += "/" + strings.Join(fragment, "/")
			}
			c.schema.ID = childID
			c.schema.IDCalc = true
		}
		c.schema.calculateID()
	}
}

type child struct {
	*RefOrSchema
	path []interface{}
}

func (s *Schema) children() (children []child) {
	push := func(s *RefOrSchema, path ...interface{}) {
		if s != nil {
			children = append(children, child{s, path})
		}
	}
	if s.AdditionalItems != nil {
		push(s.AdditionalItems.Schema, "additionalItems")
	}
	if s.Items != nil {
		push(s.Items.Items, "items")
		for i, f := range s.Items.TupleFields {
			push(f, "items", i)
		}
	}
	if s.AdditionalProperties != nil {
		push(s.AdditionalProperties.Schema, "additionalProperties")
	}
	for _, m := range []struct {
		name    string
		schemas map[string]*RefOrSchema
	}{
		{"definitions", s.Definitions},
		{"properties", s.Properties},
		{"patternProperties", s.PatternProperties},
		{"dependencies", s.Dependencies},
	} {
		for k, v := range m.schemas {
			push(v, m.name, k)
		}
	}
	for _, a := range []struct {
		name    string
		schemas []*RefOrSchema
	}{
		{"allOf", s.AllOf},
		{"anyOf", s.AnyOf},
		{"oneOf", s.OneOf},
	} {
		for i, v := range a.schemas {
			push(v, a.name, i)
		}
	}
	push(s.Not, "not")
	return
}

// String returns a simple string identifier for the schema
func (s *Schema) String() string {
	if s.ID == nil {
		return "<nil>"
	}
	return s.ID.String()
}

// ChooseType returns the best known type for this field.
func (s *Schema) ChooseType() (t SimpleType) {
	if s.Type != nil && len(*s.Type) > 0 {
		t = (*s.Type)[0]
	}
	if len(s.Properties) > 0 || s.AdditionalProperties.Present() {
		return Object // we'll assume object if it has properties
	}
	return
}

// UnmarshalJSON is custom JSON deserialization for the Schema type
func (s *Schema) UnmarshalJSON(data []byte) error {
	{
		type schema Schema

		var s2 schema
		if err := json.Unmarshal(data, &s2); err != nil {
			return fmt.Errorf("unmarshal schema: %w", err)
		}
		*s = Schema(s2)
	}

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

	for _, key := range []string{"$id", "id"} {
		idBytes, ok := s.Annotations[key]
		if !ok {
			continue
		}
		var (
			id  string
			err error
		)
		if err = json.Unmarshal(idBytes, &id); err != nil {
			return err
		}
		if s.ID, err = url.Parse(id); err != nil {
			return err
		}
		break
	}
	return nil
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
