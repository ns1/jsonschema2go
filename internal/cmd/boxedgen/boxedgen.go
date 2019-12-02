package main

import (
	"bytes"
	"go/format"
	"log"
	"os"
	"text/template"
	"unicode"
)

var tmpl = template.Must(template.New("").Parse(`
package boxed

import (
	"encoding/json"
	"errors"
)

var (
	ErrMarshalUnset = errors.New("marshalling unset var")
	ErrNullInvalid  = errors.New("null is invalid")
)

{{ range . -}}
type {{ .WrapperName }} struct {
	{{ .WrapperName }} {{ .TypeName }}
	Set bool
}

func (m {{ .WrapperName }}) MarshalJSON() ([]byte, error) {
	if !m.Set {
		return nil, ErrMarshalUnset
	}
	return json.Marshal(m.{{ .WrapperName }})
}

func (m *{{ .WrapperName }}) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return ErrNullInvalid
	}
	if err := json.Unmarshal(data, &m.{{ .WrapperName }}); err != nil {
		return err
	}
	m.Set = true
	return nil
}
{{ end -}}
`))

func main() {
	type typeInfo struct {
		WrapperName, TypeName string
	}

	var types []typeInfo
	for _, t := range []string{
		"int64",
		"float64",
		"string",
		"bool",
	} {
		t2 := []rune(t)
		t2[0] = unicode.ToUpper(t2[0])
		types = append(types, typeInfo{string(t2), t})
	}

	var w bytes.Buffer
	if err := tmpl.Execute(&w, types); err != nil {
		log.Fatal(err)
	}

	data, err := format.Source(w.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Create("boxed/boxed.go")
	if err != nil {
		log.Fatal(err)
	}
	func() {
		defer f.Close()
		_, err = f.Write(data)
		return
	}()
	if err != nil {
		log.Fatal(err)
	}
}
